#!/usr/bin/env python3
"""Lexis launcher.

  python3 run.py [up] [--build] [--open]   поднять контейнеры и дождаться готовности
  python3 run.py down                      остановить контейнеры (данные сохраняются)
  python3 run.py restart                   перезапустить
  python3 run.py logs [service]            логи (app | db), follow
  python3 run.py status                    состояние контейнеров и API
"""
from __future__ import annotations

import argparse
import platform
import shutil
import subprocess
import sys
import time
import urllib.error
import urllib.request
import webbrowser
from pathlib import Path

ROOT = Path(__file__).resolve().parent
ENV_FILE = ROOT / ".env"
ENV_EXAMPLE = ROOT / ".env.example"

HEALTH_TIMEOUT = 120
DOCKER_START_TIMEOUT = 90
IS_TTY = sys.stdout.isatty()
IS_MAC = platform.system() == "Darwin"


# ---------- output ----------

def _color(code: str):
    return lambda s: f"\033[{code}m{s}\033[0m" if IS_TTY else str(s)


green, red, yellow, bold = (_color(c) for c in ("32", "31", "33", "1"))


def info(msg: str) -> None:
    print(f"{green('▸')} {msg}")


def warn(msg: str) -> None:
    print(f"{yellow('!')} {msg}")


def fail(msg: str, code: int = 1) -> None:
    print(f"{red('✗')} {msg}", file=sys.stderr)
    sys.exit(code)


def clear_line() -> None:
    if IS_TTY:
        print("\r\033[K", end="", flush=True)


def wait_for(pred, timeout: int, label: str) -> bool:
    frames = "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏"
    start = time.monotonic()
    i = 0
    while time.monotonic() - start < timeout:
        if pred():
            clear_line()
            return True
        if IS_TTY:
            elapsed = int(time.monotonic() - start)
            print(f"\r{frames[i % len(frames)]} {label}... {elapsed}s", end="", flush=True)
        i += 1
        time.sleep(0.5)
    clear_line()
    return False


# ---------- env ----------

def load_env() -> dict:
    if not ENV_FILE.exists() and ENV_EXAMPLE.exists():
        shutil.copy(ENV_EXAMPLE, ENV_FILE)
        info("Создан .env из .env.example")
    env: dict = {}
    if ENV_FILE.exists():
        for line in ENV_FILE.read_text(encoding="utf-8").splitlines():
            line = line.strip()
            if not line or line.startswith("#") or "=" not in line:
                continue
            key, value = line.split("=", 1)
            env[key.strip()] = value.strip().strip('"').strip("'")
    return env


def port(env: dict) -> str:
    return env.get("LEXIS_PORT", "8090")


def app_url(env: dict) -> str:
    return f"http://localhost:{port(env)}"


def api_url(env: dict) -> str:
    # 127.0.0.1, а не localhost: порт проброшен только на IPv4
    return f"http://127.0.0.1:{port(env)}"


# ---------- docker ----------

def compose(*args: str, check: bool = True) -> subprocess.CompletedProcess:
    return subprocess.run(["docker", "compose", *args], cwd=ROOT, check=check, text=True)


def docker_running() -> bool:
    r = subprocess.run(["docker", "info"], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    return r.returncode == 0


def ensure_docker() -> None:
    if shutil.which("docker") is None:
        fail("Docker не найден. Установи Docker Desktop: https://www.docker.com/products/docker-desktop/")
    if docker_running():
        return
    if IS_MAC:
        info("Docker Desktop не запущен — запускаю")
        subprocess.run(["open", "-a", "Docker"], check=False)
        if wait_for(docker_running, DOCKER_START_TIMEOUT, "Жду Docker"):
            return
    fail("Docker daemon недоступен. Запусти Docker Desktop и повтори.")


def healthy(env: dict) -> bool:
    try:
        with urllib.request.urlopen(f"{api_url(env)}/healthz", timeout=2) as r:
            return r.status == 200
    except (urllib.error.URLError, ConnectionError, OSError):
        return False


def wait_healthy_or_die(env: dict) -> None:
    if wait_for(lambda: healthy(env), HEALTH_TIMEOUT, "Жду готовности приложения"):
        return
    warn(f"Приложение не ответило за {HEALTH_TIMEOUT}s. Последние логи app:")
    compose("logs", "--tail", "40", "app", check=False)
    sys.exit(1)


def print_ready(env: dict, open_browser: bool) -> None:
    print()
    print(f"  {bold('Lexis готов →')} {green(app_url(env))}")
    print()
    if open_browser:
        webbrowser.open(app_url(env))


# ---------- commands: containers ----------

def cmd_up(args) -> None:
    ensure_docker()
    env = load_env()
    up_args = ["up", "-d"]
    if args.build:
        up_args.append("--build")
    info("Поднимаю контейнеры" + (" (с пересборкой)" if args.build else ""))
    compose(*up_args)
    wait_healthy_or_die(env)
    print_ready(env, args.open)


def cmd_down(args) -> None:
    ensure_docker()
    compose("down")
    info("Контейнеры остановлены, данные в volume сохранены")


def cmd_restart(args) -> None:
    ensure_docker()
    env = load_env()
    info("Перезапускаю")
    compose("restart")
    wait_healthy_or_die(env)
    print_ready(env, False)


def cmd_logs(args) -> None:
    ensure_docker()
    extra = [args.service] if args.service else []
    compose("logs", "-f", "--tail", "100", *extra, check=False)


def cmd_status(args) -> None:
    ensure_docker()
    env = load_env()
    compose("ps", check=False)
    print()
    api = green("ok") if healthy(env) else red("недоступен")
    print(f"  API {api_url(env)}: {api}")


# ---------- main ----------

def main() -> None:
    argv = sys.argv[1:]
    if not argv or (argv[0].startswith("-") and argv[0] not in ("-h", "--help")):
        argv = ["up", *argv]

    parser = argparse.ArgumentParser(prog="run.py", description="Lexis launcher")
    sub = parser.add_subparsers(dest="cmd", required=True)

    up = sub.add_parser("up", help="поднять и дождаться готовности")
    up.add_argument("--build", action="store_true", help="пересобрать образ")
    up.add_argument("--open", action="store_true", help="открыть в браузере")
    up.set_defaults(func=cmd_up)

    sub.add_parser("down", help="остановить").set_defaults(func=cmd_down)
    sub.add_parser("restart", help="перезапустить").set_defaults(func=cmd_restart)

    logs = sub.add_parser("logs", help="логи контейнеров")
    logs.add_argument("service", nargs="?", choices=["app", "db"])
    logs.set_defaults(func=cmd_logs)

    sub.add_parser("status", help="состояние").set_defaults(func=cmd_status)

    args = parser.parse_args(argv)
    try:
        args.func(args)
    except subprocess.CalledProcessError as e:
        fail(f"Команда упала: {' '.join(map(str, e.cmd))} (код {e.returncode})", e.returncode or 1)
    except KeyboardInterrupt:
        print()
        sys.exit(130)


if __name__ == "__main__":
    main()
