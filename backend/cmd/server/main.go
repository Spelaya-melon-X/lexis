// Command lexis — сервер Lexis: API + встроенный фронт.
//
//	lexis              — запустить сервер
//	lexis healthcheck  — GET /healthz, exit 0/1 (для healthcheck в distroless, где нет curl)
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"lexis/internal/config"
	"lexis/internal/platform/clock"
	"lexis/internal/platform/db"
	"lexis/internal/platform/eventbus"
)

const shutdownTimeout = 10 * time.Second

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck(os.Getenv("LEXIS_HTTP_ADDR")))
	}

	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)
	if err := run(log); err != nil {
		log.Error("server stopped with error", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}

	// Ctrl+C / docker stop → ctx отменяется → graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Open(ctx, cfg.DBDSN)
	if err != nil {
		return err
	}
	defer pool.Close() // defer-ы срабатывают в обратном порядке: пул закроется последним

	if err := db.Migrate(ctx, pool, log); err != nil {
		return err
	}

	clk := clock.NewReal(cfg.Location)
	bus := eventbus.New(log)
	defer bus.Close() // после остановки HTTP: подписчики дописывают очередь, пока пул ещё жив

	handler, err := newRouter(ctx, pool, clk, bus)
	if err != nil {
		return err
	}
	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second, // защита от «медленных» клиентов, держащих соединение
	}

	errc := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", cfg.HTTPAddr, "tz", cfg.Location.String())
		errc <- srv.ListenAndServe()
	}()

	select {
	case err := <-errc: // не смогли занять порт и т.п.
		return fmt.Errorf("http server: %w", err)
	case <-ctx.Done():
	}

	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("http shutdown: %w", err)
	}
	if err := <-errc; !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("http server: %w", err)
	}
	return nil
}
