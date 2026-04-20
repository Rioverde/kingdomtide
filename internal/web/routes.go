package web

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// buildRouter constructs the chi router, mounts middleware, and wires all routes.
func buildRouter(s *Server) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Logger)
	r.Use(middleware.Compress(5))

	r.Get("/", s.handleIndex)

	r.Route("/api", func(r chi.Router) {
		r.Get("/meta", s.handleAPIMeta)
		r.Get("/chunk", s.handleAPIChunk)
	})

	r.Handle("/static/*", staticFileServer(s.cfg))
	r.Handle("/tiles/*", tilesFileServer(s.cfg.TilesDir))

	return r
}
