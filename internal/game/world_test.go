package game

import (
	"reflect"
	"testing"
)

// stubLandmarkSource is a minimal LandmarkSource used to verify the
// World delegates LandmarksIn to its configured backend. It records
// the queried coord so the test can assert the World forwarded the
// argument unchanged.
type stubLandmarkSource struct {
	got  SuperChunkCoord
	out  []Landmark
	hits int
}

func (s *stubLandmarkSource) LandmarksIn(sc SuperChunkCoord) []Landmark {
	s.got = sc
	s.hits++
	return s.out
}

func TestWorldLandmarksInNilSource(t *testing.T) {
	w := newTestWorld(testTiles{})
	got := w.LandmarksIn(SuperChunkCoord{X: 3, Y: -2})
	if got != nil {
		t.Fatalf("LandmarksIn with nil source = %v, want nil", got)
	}
}

func TestWorldLandmarksInDelegation(t *testing.T) {
	want := []Landmark{
		{Coord: Position{X: 10, Y: 20}, Kind: LandmarkTower},
		{Coord: Position{X: 30, Y: 40}, Kind: LandmarkShrine},
	}
	stub := &stubLandmarkSource{out: want}
	w := NewWorldFromSource(testTiles{}, WithLandmarkSource(stub))

	sc := SuperChunkCoord{X: 7, Y: 11}
	got := w.LandmarksIn(sc)

	if stub.hits != 1 {
		t.Fatalf("source hits = %d, want 1", stub.hits)
	}
	if stub.got != sc {
		t.Fatalf("source received sc = %+v, want %+v", stub.got, sc)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("LandmarksIn = %+v, want %+v", got, want)
	}
}

func TestNewWorldInBoundsAlwaysTrue(t *testing.T) {
	w := newTestWorld(testTiles{})
	if !w.InBounds(Position{X: -1e6, Y: 1e6}) {
		t.Fatalf("expected infinite world to report InBounds for any coord")
	}
}

func TestPlayersDefensiveCopyAndSort(t *testing.T) {
	w := newTestWorld(testTiles{})
	for _, id := range []string{"charlie", "alice", "bob"} {
		if _, err := w.ApplyCommand(JoinCmd{PlayerID: id, Name: id}); err != nil {
			t.Fatalf("join %q: %v", id, err)
		}
	}
	got := w.Players()
	if len(got) != 3 {
		t.Fatalf("len(Players()) = %d, want 3", len(got))
	}
	wantOrder := []string{"alice", "bob", "charlie"}
	for i, id := range wantOrder {
		if got[i].ID != id {
			t.Fatalf("Players()[%d].ID = %q, want %q", i, got[i].ID, id)
		}
	}
	got[0] = nil
	again := w.Players()
	if again[0] == nil || again[0].ID != "alice" {
		t.Fatalf("Players() not defensively copied: %+v", again)
	}
}

func TestTerrainPassable(t *testing.T) {
	passable := map[Terrain]bool{
		TerrainPlains:    true,
		TerrainGrassland: true,
		TerrainMeadow:    true,
		TerrainBeach:     true,
		TerrainSavanna:   true,
		TerrainDesert:    true,
		TerrainSnow:      true,
		TerrainTundra:    true,
		TerrainTaiga:     true,
		TerrainForest:    true,
		TerrainJungle:    true,
		TerrainHills:     true,
		TerrainDeepOcean: false,
		TerrainOcean:     false,
		TerrainMountain:  false,
		TerrainSnowyPeak: false,
	}
	for _, terr := range AllTerrains() {
		want, ok := passable[terr]
		if !ok {
			t.Fatalf("test is missing an expectation for terrain %q", terr)
		}
		if got := terr.Passable(); got != want {
			t.Fatalf("Terrain(%q).Passable() = %v, want %v", terr, got, want)
		}
	}
	if Terrain("").Passable() {
		t.Fatalf(`Terrain("").Passable() = true, want false`)
	}
	if Terrain("garbage").Passable() {
		t.Fatalf(`Terrain("garbage").Passable() = true, want false`)
	}
}
