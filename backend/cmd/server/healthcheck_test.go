package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthURL(t *testing.T) {
	tests := []struct{ addr, want string }{
		{"", "http://127.0.0.1:8080/healthz"},
		{":8080", "http://127.0.0.1:8080/healthz"},
		{"0.0.0.0:9000", "http://127.0.0.1:9000/healthz"},
		{"localhost:7000", "http://127.0.0.1:7000/healthz"},
		{"garbage", "http://127.0.0.1:8080/healthz"},
	}
	for _, tt := range tests {
		if got := healthURL(tt.addr); got != tt.want {
			t.Errorf("%q: got %q, want %q", tt.addr, got, tt.want)
		}
	}
}

func TestHealthcheck(t *testing.T) {
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(status)
	}))
	defer srv.Close()
	_, port, _ := net.SplitHostPort(srv.Listener.Addr().String())

	if code := healthcheck(":" + port); code != 0 {
		t.Errorf("healthy server: exit %d", code)
	}
	status = http.StatusServiceUnavailable
	if code := healthcheck(":" + port); code != 1 {
		t.Errorf("unhealthy server: exit %d", code)
	}
	srv.Close()
	if code := healthcheck(":" + port); code != 1 {
		t.Errorf("down server: exit %d", code)
	}
}
