package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"time"
)

// healthcheck — `lexis healthcheck`: в distroless-образе нет curl/wget, поэтому
// compose-healthcheck вызывает сам бинарник. 0 — здоров, 1 — нет.
func healthcheck(addr string) int {
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(healthURL(addr))
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "healthcheck: status", resp.StatusCode)
		return 1
	}
	return 0
}

// healthURL: ":8080" / "0.0.0.0:8080" / "" → http://127.0.0.1:8080/healthz.
// Проверяем изнутри контейнера, поэтому стучимся на loopback, а не на адрес прослушивания.
func healthURL(addr string) string {
	port := "8080"
	if _, p, err := net.SplitHostPort(addr); err == nil && p != "" {
		port = p
	}
	return "http://" + net.JoinHostPort("127.0.0.1", port) + "/healthz"
}
