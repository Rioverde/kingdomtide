// Package web serves the game world as a rendered hex map over HTTP.
package web

import (
	"fmt"
	"html/template"
	"net/http"
	"sync"

	"github.com/Rioverde/gongeons/internal/game"
)

// Compile-time assertion: *Server must satisfy http.Handler via Handler().
// We use the indirect form because Server itself is not an http.Handler —
// its Handler() method returns one.
var _ interface{ Handler() http.Handler } = (*Server)(nil)

// Config configures the HTTP server.
type Config struct {
	// TilesDir is the filesystem path to the directory containing terrain tile PNGs.
	TilesDir string

	// Seed is the seed used for the initial world. When zero a per-request seed must be supplied.
	Seed int64
}

// Server owns the current world and serves it on each HTTP request. The world can be
// regenerated concurrently by passing ?seed=N in the query string.
type Server struct {
	cfg  Config
	tmpl *template.Template

	mu    sync.RWMutex
	world *game.World
	seed  int64
}

// NewServer parses the embedded HTML template and generates the initial world.
func NewServer(cfg Config) (*Server, error) {
	tmpl, err := template.ParseFS(staticFS, "static/index.html.tmpl")
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}
	s := &Server{cfg: cfg, tmpl: tmpl}
	s.regenerate(cfg.Seed)
	return s, nil
}

// Handler returns the HTTP handler serving the UI, JSON API, static assets and tile PNGs.
func (s *Server) Handler() http.Handler {
	return buildRouter(s)
}

// regenerate rebuilds the infinite chunked world from the given seed. Zero reuses the
// previously stored seed, which matters on first boot when cfg.Seed may be unset.
func (s *Server) regenerate(seed int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if seed == 0 {
		seed = s.seed
	}
	s.world = game.NewWorld(seed)
	s.seed = seed
}
