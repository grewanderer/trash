package server

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed webui/*
var uiFS embed.FS

func (a *App) RegisterWebUI(prefix string) {
	if prefix == "" {
		prefix = "/ui/"
	}
	// нормализуем
	base := strings.TrimSuffix(prefix, "/")
	slash := base + "/"

	sub, err := fs.Sub(uiFS, "webui")
	if err != nil {
		// если webui нет в бинаре, лучше сразу паникнуть — иначе будет 404/301
		panic(err)
	}

	// 1) /ui -> /ui/ (одноразовый редирект)
	a.Router.HandleFunc(base, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, slash, http.StatusFound) // 302
	}).Methods(http.MethodGet)

	// 2) /ui/ -> отдать index.html БЕЗ FileServer (чтобы устранить 301-луп)
	a.Router.HandleFunc(slash, func(w http.ResponseWriter, r *http.Request) {
		b, err := fs.ReadFile(sub, "index.html")
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("ui: index.html not embedded; ensure server/webui/* exists and rebuild"))
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(b)
	}).Methods(http.MethodGet)

	// 3) Остальная статика: /ui/<files>
	fileServer := http.StripPrefix(slash, http.FileServer(http.FS(sub)))
	a.Router.PathPrefix(slash).Handler(fileServer)

	// 4) Корень редиректим на UI (удобно)
	a.Router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, slash, http.StatusFound)
	})
}
