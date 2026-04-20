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
// filtering out .tmpl files.
func staticFileServer() http.Handler {
	return http.FileServer(http.FS(staticOnlyFS{}))
}

// tilesFileServer returns an http.Handler that serves terrain PNG sprites from dir,
// stripping the /tiles/ prefix before looking up files on disk.
func tilesFileServer(dir string) http.Handler {
	return http.StripPrefix("/tiles/", http.FileServer(http.Dir(dir)))
}
