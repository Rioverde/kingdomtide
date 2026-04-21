package game

import "testing"

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
