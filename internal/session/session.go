package session

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/ssh"
	bm "github.com/charmbracelet/wish/bubbletea"

	"github.com/Rioverde/gongeons/internal/server"
	"github.com/Rioverde/gongeons/internal/ui"
)

// Handler returns the wish/bubbletea middleware handler bound to svc.
// Every incoming SSH connection opens its own Bubble Tea Model that
// talks directly to svc — same world, same tick loop, no gRPC hop.
//
// The model is constructed via ui.NewSession rather than ui.New so
// command plumbing (Join / Move / Viewport) and event ingestion
// (tick-driven SessionEvents) route through in-process Service calls
// instead of a gRPC stream. The returned tea.ProgramOptions mirror the
// stand-alone client's setup: alt-screen + mouse cell motion.
func Handler(svc *server.Service) bm.Handler {
	return func(s ssh.Session) (tea.Model, []tea.ProgramOption) {
		_, _, active := s.Pty()
		if !active {
			// The activeterm middleware gates the bubbletea middleware
			// and already rejects non-PTY sessions. Guard here too so a
			// misconfigured wire-up (middleware chain reordered) fails
			// fast with a nil model rather than a NPE deep in the Model
			// render path.
			return nil, nil
		}
		m := ui.NewSession(s.Context(), svc, s)
		return m, []tea.ProgramOption{
			tea.WithAltScreen(),
			tea.WithMouseCellMotion(),
		}
	}
}
