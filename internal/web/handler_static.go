package web

import (
	"io/fs"
	"net/http"
	"strings"
)

// staticOnlyFS exposes the embedded static directory while hiding server-only files such as
// Go templates from the public URL space.
type staticOnlyFS struct{}

func (staticOnlyFS) Open(name string) (fs.File, error) {
	if strings.HasSuffix(name, ".tmpl") {
		return nil, fs.ErrNotExist
	}
	return staticFS.Open(name)
}

// staticFileServer returns an http.Handler that serves embedded static assets,
// filtering out .tmpl files. When cfg.NoCache is true, responses carry
// Cache-Control: no-store to prevent browsers from serving stale assets during
// development. In production the default browser caching behaviour applies.
func staticFileServer(cfg Config) http.Handler {
	h := http.Handler(http.FileServer(http.FS(staticOnlyFS{})))
	if cfg.NoCache {
		h = noStore(h)
	}
	return h
}

// tilesFileServer returns an http.Handler that serves terrain PNG sprites from dir,
// stripping the /tiles/ prefix before looking up files on disk.
func tilesFileServer(dir string) http.Handler {
	return http.StripPrefix("/tiles/", http.FileServer(http.Dir(dir)))
}

// noStore wraps h so its responses carry Cache-Control: no-store. During development
// this guarantees browsers never serve a stale copy of the embedded JS or CSS, which is
// otherwise the most common source of "I changed the code but nothing changed" reports.
func noStore(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		h.ServeHTTP(w, r)
	})
}
