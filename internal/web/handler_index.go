package web

import (
	"net/http"
	"strconv"
)

// indexViewModel carries the minimal data the shell template needs.
type indexViewModel struct {
	Seed int64
}

// handleIndex renders the shell HTML page. If ?seed=N is present the world is regenerated
// before rendering. A non-integer seed value returns 400. A path other than "/" returns 404.
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	if raw := r.URL.Query().Get("seed"); raw != "" {
		seed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			http.Error(w, "invalid seed parameter", http.StatusBadRequest)
			return
		}
		s.RegenerateWith(seed)
	}

	vm := indexViewModel{Seed: s.seed.Load()}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.Execute(w, vm); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
