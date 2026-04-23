package world

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/naming/parts"
)

// TestWorldRegionAtPlaceholder lives in the game package (not world/) because
// it exercises World.RegionAt — a method on the World aggregate, which still
// resides here until the aggregate moves into world/ in a later step.
func TestWorldRegionAtPlaceholder(t *testing.T) {
	// Without a RegionSource, World.RegionAt must still return a sane
	// Region: character Normal, and the anchor/coord that AnchorAt would
	// return for the queried position.
	w := newTestWorld(testTiles{})
	p := geom.Position{X: 10, Y: 20}
	r := w.RegionAt(p)
	if r.Character != RegionNormal {
		t.Fatalf("placeholder RegionAt character = %s, want normal", r.Character)
	}
	wantAnchor, wantSC := geom.AnchorAt(w.Seed(), p.X, p.Y)
	if r.Coord != wantSC || r.Anchor != wantAnchor {
		t.Fatalf("placeholder RegionAt coord/anchor mismatch: got (%+v, %+v), want (%+v, %+v)",
			r.Anchor, r.Coord, wantAnchor, wantSC)
	}
}

// stubRegionSource satisfies RegionSource with a trivial per-coord tag
// so tests can verify that World.RegionAt delegates to the configured
// source. The stubBodySeed sentinel proves the tag travels through.
type stubRegionSource struct {
	seen map[geom.SuperChunkCoord]int
}

const stubBodySeed int64 = 0x5ca1ab1e

func (s *stubRegionSource) RegionAt(sc geom.SuperChunkCoord) Region {
	if s.seen != nil {
		s.seen[sc]++
	}
	return Region{
		Coord:     sc,
		Name:      parts.Parts{BodySeed: stubBodySeed},
		Character: RegionWild,
	}
}

func TestWorldRegionAtDelegates(t *testing.T) {
	src := &stubRegionSource{seen: make(map[geom.SuperChunkCoord]int)}
	w := NewWorldFromSource(testTiles{}, WithSeed(17), WithRegionSource(src))
	if w.Seed() != 17 {
		t.Fatalf("Seed() = %d, want 17", w.Seed())
	}
	r := w.RegionAt(geom.Position{X: 1, Y: 2})
	if r.Character != RegionWild || r.Name.BodySeed != stubBodySeed {
		t.Fatalf("RegionAt did not delegate to stub source: %+v", r)
	}
	_, sc := geom.AnchorAt(17, 1, 2)
	if src.seen[sc] != 1 {
		t.Fatalf("expected one RegionAt call for %+v, got %d", sc, src.seen[sc])
	}
}
