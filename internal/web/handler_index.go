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
// before rendering. A path other than "/" returns 404.
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	if raw := r.URL.Query().Get("seed"); raw != "" {
		if seed, err := strconv.ParseInt(raw, 10, 64); err == nil {
			s.regenerate(seed)
		}
	}

	s.mu.RLock()
	vm := indexViewModel{Seed: s.seed}
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.Execute(w, vm); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
