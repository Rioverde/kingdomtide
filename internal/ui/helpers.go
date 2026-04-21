package ui

// Helpers for the bubbletea UI layer: small, composable primitives shared by
// the render, network, and mapping paths. Anything that reads or mutates
// Model stays in its original file; only pure utilities live here.

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	pb "github.com/Rioverde/gongeons/internal/proto"
)

// renderPanel is the shared skeleton for bordered panels with a header
// line and a list of items. Returns the empty-state label when src is
// empty; otherwise formats each item and joins with newlines.
func renderPanel[T any](
	header, empty string,
	style lipgloss.Style,
	src []T,
	format func(T) string,
) string {
	if len(src) == 0 {
		return style.Render(header + "\n" + empty)
	}
	var b strings.Builder
	b.WriteString(header + "\n")
	for _, item := range src {
		b.WriteString(format(item))
		b.WriteByte('\n')
	}
	return style.Render(strings.TrimRight(b.String(), "\n"))
}

// sortedMapValues returns the values of m as a slice, ordered by key.
// Stable output for rendering; predictable for tests.
func sortedMapValues[K cmp.Ordered, V any](m map[K]V) []V {
	keys := make([]K, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	out := make([]V, 0, len(keys))
	for _, k := range keys {
		out = append(out, m[k])
	}
	return out
}

// sendNonBlocking returns a tea.Cmd that tries to queue msg on outbox
// without blocking. Returns nil on success; netErrorMsg if the channel
// is full (which means the writer goroutine is dead and the session is
// doomed — same semantics as the previous hand-rolled helpers).
func sendNonBlocking(outbox chan<- *pb.ClientMessage, msg *pb.ClientMessage, label string) tea.Cmd {
	return func() tea.Msg {
		select {
		case outbox <- msg:
			return nil
		default:
			return netErrorMsg{Err: fmt.Errorf("outbox full on %s", label)}
		}
	}
}
