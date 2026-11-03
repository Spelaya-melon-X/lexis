// Package web отдаёт собранный фронт (Vite → web/dist), встроенный в бинарник.
package web

import (
	"bytes"
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"
)

// all: — чтобы попали и файлы с точкой/подчёркиванием (Vite кладёт такие в assets).
// В репозитории dist содержит только .gitkeep: go:embed требует существующий каталог,
// а настоящий фронт кладёт туда Dockerfile (или `npm run build` локально).
//
//go:embed all:dist
var dist embed.FS

// Handler — статика фронта из бинарника.
func Handler() http.Handler {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err) // "dist" зашит в go:embed выше — сюда не попасть
	}
	return newHandler(sub)
}

// newHandler отдельно от Handler — чтобы тестировать на fstest.MapFS.
//
// Роутера на фронте нет (табы + #hash), но неизвестный путь всё равно отдаёт index.html:
// так обновление страницы на любом URL не даёт 404.
func newHandler(fsys fs.FS) http.Handler {
	if _, err := fs.Stat(fsys, "index.html"); err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "frontend is not built: cd frontend && npm run build (or use the Vite dev server)", http.StatusServiceUnavailable)
		})
	}
	files := http.FileServerFS(fsys)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "" || name == "index.html" {
			serveIndex(w, r, fsys)
			return
		}
		if _, err := fs.Stat(fsys, name); err != nil {
			serveIndex(w, r, fsys)
			return
		}
		// У Vite имена в assets/ содержат хеш содержимого — их можно кэшировать навсегда.
		if strings.HasPrefix(name, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		files.ServeHTTP(w, r)
	})
}

// serveIndex — index.html без кэша: после обновления приложения браузер должен
// сразу получить ссылки на новые assets.
//
// ServeContent, а не ServeFileFS: тот редиректит ".../index.html" на "./" и отвечает 400
// на путь с "..", а нам на любом пути нужен просто index.
func serveIndex(w http.ResponseWriter, r *http.Request, fsys fs.FS) {
	data, err := fs.ReadFile(fsys, "index.html")
	if err != nil {
		http.Error(w, "index.html not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(data))
}
