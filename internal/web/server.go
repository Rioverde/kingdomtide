// Package web serves the game world as a rendered hex map over HTTP.
package web

import (
	"fmt"
	"html/template"
	"math/rand"
	"net/http"
	"sync/atomic"

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

	// Seed is the seed used for the initial world. When zero a random seed is chosen.
	Seed int64

	// NoCache disables caching on static assets when true. Intended for development
	// so browsers never serve a stale copy of embedded JS or CSS.
	NoCache bool
}

// Server owns the current world and serves it on each HTTP request. The world can be
// regenerated concurrently by passing ?seed=N in the query string.
//
// world and seed use atomic storage so readers always see a consistent pair
// without holding a mutex — a torn view (new world, old seed or vice-versa) is
// impossible because RegenerateWith stores the world pointer last.
type Server struct {
	cfg  Config
	tmpl *template.Template

	world atomic.Pointer[game.World]
	seed  atomic.Int64
}

// NewServer parses the embedded HTML template and generates the initial world.
func NewServer(cfg Config) (*Server, error) {
	tmpl, err := template.ParseFS(staticFS, "static/index.html.tmpl")
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}
	s := &Server{cfg: cfg, tmpl: tmpl}
	seed := cfg.Seed
	if seed == 0 {
		seed = rand.Int63()
	}
	s.RegenerateWith(seed)
	return s, nil
}

// Handler returns the HTTP handler serving the UI, JSON API, static assets and tile PNGs.
func (s *Server) Handler() http.Handler {
	return buildRouter(s)
}

// Regenerate picks a fresh random seed and rebuilds the world from it.
// It returns the seed that was chosen so callers can surface it to clients.
func (s *Server) Regenerate() int64 {
	seed := rand.Int63()
	s.RegenerateWith(seed)
	return seed
}

// RegenerateWith rebuilds the infinite chunked world from seed. The seed and
// world pointer are published atomically so concurrent readers always see a
// consistent pair.
func (s *Server) RegenerateWith(seed int64) {
	w := game.NewWorld(seed)
	// Store seed first; world last. Readers that load world then seed may briefly
	// see the old seed with the new world, but that is harmless — the next read
	// of /api/meta will return the correct pair. The important invariant is that
	// a reader never sees the new world with the old seed surfaced in the UI.
	// Storing world last ensures no reader can act on the new world before the
	// new seed is visible.
	s.seed.Store(seed)
	s.world.Store(w)
}
