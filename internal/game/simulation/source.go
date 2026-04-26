package simulation

import (
	"sort"

	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/polity"
)

// Source is the polity.SettlementSource implementation backing the
// finalized simulation Result. Built once at finalize time; safe for
// concurrent read; no further mutation.
type Source struct {
	camps    []polity.Camp
	hamlets  []polity.Hamlet
	villages []polity.Village

	bySC map[geom.SuperChunkCoord][]polity.Place
}

// SettlementSource builds and returns the polity.SettlementSource
// view of the final state. Only callable AFTER Run completes;
// snapshots collected during the simulation are unaffected.
func (r *Result) SettlementSource() *Source {
	src := &Source{
		bySC: make(map[geom.SuperChunkCoord][]polity.Place),
	}

	for _, id := range sortedIDs(r.settlements) {
		place := r.settlements[id]
		switch p := place.(type) {
		case *polity.Camp:
			src.camps = append(src.camps, *p)
		case *polity.Hamlet:
			src.hamlets = append(src.hamlets, *p)
		case *polity.Village:
			src.villages = append(src.villages, *p)
		}
	}

	// Stable sort each tier slice by Position (Y, X) lex order.
	sort.Slice(src.camps, func(i, j int) bool {
		return lessPos(src.camps[i].Position, src.camps[j].Position)
	})
	sort.Slice(src.hamlets, func(i, j int) bool {
		return lessPos(src.hamlets[i].Position, src.hamlets[j].Position)
	})
	sort.Slice(src.villages, func(i, j int) bool {
		return lessPos(src.villages[i].Position, src.villages[j].Position)
	})

	// Build per-SC index. Pointers into the tier slices are stable because
	// tier slices are not resized after this point.
	for i := range src.camps {
		pos := src.camps[i].Position
		sc := geom.WorldToSuperChunk(pos.X, pos.Y)
		src.bySC[sc] = append(src.bySC[sc], &src.camps[i])
	}
	for i := range src.hamlets {
		pos := src.hamlets[i].Position
		sc := geom.WorldToSuperChunk(pos.X, pos.Y)
		src.bySC[sc] = append(src.bySC[sc], &src.hamlets[i])
	}
	for i := range src.villages {
		pos := src.villages[i].Position
		sc := geom.WorldToSuperChunk(pos.X, pos.Y)
		src.bySC[sc] = append(src.bySC[sc], &src.villages[i])
	}

	return src
}

// PlacesIn returns every settlement whose anchor falls inside the
// given super-chunk, sorted by Position.
func (s *Source) PlacesIn(sc geom.SuperChunkCoord) []polity.Place {
	return s.bySC[sc]
}

// AllCamps returns every camp in (Y, X) lex order.
func (s *Source) AllCamps() []polity.Camp { return s.camps }

// AllHamlets returns every hamlet in (Y, X) lex order.
func (s *Source) AllHamlets() []polity.Hamlet { return s.hamlets }

// AllVillages returns every village in (Y, X) lex order.
func (s *Source) AllVillages() []polity.Village { return s.villages }

// Compile-time interface assertion.
var _ polity.SettlementSource = (*Source)(nil)

// sortedIDs returns the keys of the settlements map in ascending order.
func sortedIDs(m map[polity.SettlementID]polity.Place) []polity.SettlementID {
	ids := make([]polity.SettlementID, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// lessPos reports whether position a sorts before b in (Y, X) lex order.
func lessPos(a, b geom.Position) bool {
	if a.Y != b.Y {
		return a.Y < b.Y
	}
	return a.X < b.X
}
