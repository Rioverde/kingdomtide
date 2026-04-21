package game

import "testing"

func TestNewWorldPanicsOnBadDimensions(t *testing.T) {
	cases := []struct {
		name string
		w, h int
	}{
		{"zero width", 0, 5},
		{"zero height", 5, 0},
		{"negative width", -1, 5},
		{"negative height", 5, -3},
		{"both zero", 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatalf("NewWorld(%d,%d) did not panic", tc.w, tc.h)
				}
			}()
			_ = NewWorld(tc.w, tc.h)
		})
	}
}

func TestNewWorldAllPlains(t *testing.T) {
	w := NewWorld(3, 2)
	if w.Width() != 3 || w.Height() != 2 {
		t.Fatalf("dimensions: %dx%d, want 3x2", w.Width(), w.Height())
	}
	for y := range w.Height() {
		for x := range w.Width() {
			tile, ok := w.TileAt(Position{X: x, Y: y})
			if !ok {
				t.Fatalf("TileAt(%d,%d) !ok", x, y)
			}
			if tile.Terrain != TerrainPlains {
				t.Fatalf("tile(%d,%d).Terrain = %q, want plains", x, y, tile.Terrain)
			}
			if tile.Occupant != nil {
				t.Fatalf("tile(%d,%d) unexpectedly has Occupant", x, y)
			}
		}
	}
}

func TestNewMockWorldShape(t *testing.T) {
	w := NewMockWorld()
	if w.Width() != 20 || w.Height() != 10 {
		t.Fatalf("dimensions: %dx%d, want 20x10", w.Width(), w.Height())
	}

	// Border is mountain.
	corners := []Position{
		{0, 0}, {19, 0}, {0, 9}, {19, 9},
		{5, 0}, {0, 5}, {19, 4}, {10, 9},
	}
	for _, p := range corners {
		tile, ok := w.TileAt(p)
		if !ok {
			t.Fatalf("border tile %+v missing", p)
		}
		if tile.Terrain != TerrainMountain {
			t.Fatalf("border tile %+v terrain = %q, want mountain", p, tile.Terrain)
		}
	}

	// Water rectangle (5..7, 4..5) — matches the OOO block in mockLayout.
	for y := 4; y <= 5; y++ {
		for x := 5; x <= 7; x++ {
			tile, _ := w.TileAt(Position{X: x, Y: y})
			if tile.Terrain != TerrainOcean {
				t.Fatalf("ocean tile (%d,%d) = %q, want ocean", x, y, tile.Terrain)
			}
		}
	}

	// Spawn-zone corner (1,1) is forest — passable.
	if tile, _ := w.TileAt(Position{X: 1, Y: 1}); tile.Terrain != TerrainForest {
		t.Fatalf("spawn anchor (1,1) = %q, want forest", tile.Terrain)
	}

	// Grassland strip appears at (7..10, 1..2).
	if tile, _ := w.TileAt(Position{X: 7, Y: 1}); tile.Terrain != TerrainGrassland {
		t.Fatalf("grassland (7,1) = %q, want grassland", tile.Terrain)
	}

	// Hills cluster on columns 14..16.
	if tile, _ := w.TileAt(Position{X: 14, Y: 1}); tile.Terrain != TerrainHills {
		t.Fatalf("hills (14,1) = %q, want hills", tile.Terrain)
	}
}

func TestInBounds(t *testing.T) {
	w := NewWorld(4, 3)
	inCases := []Position{{0, 0}, {3, 2}, {1, 1}}
	outCases := []Position{{-1, 0}, {0, -1}, {4, 0}, {0, 3}, {10, 10}}
	for _, p := range inCases {
		if !w.InBounds(p) {
			t.Fatalf("InBounds(%+v) = false, want true", p)
		}
	}
	for _, p := range outCases {
		if w.InBounds(p) {
			t.Fatalf("InBounds(%+v) = true, want false", p)
		}
	}
}

func TestTileAtOutOfBounds(t *testing.T) {
	w := NewWorld(2, 2)
	tile, ok := w.TileAt(Position{X: 5, Y: 5})
	if ok {
		t.Fatalf("TileAt out-of-bounds: ok = true, want false")
	}
	if tile != (Tile{}) {
		t.Fatalf("TileAt out-of-bounds returned non-zero tile: %+v", tile)
	}
}

func TestPlayersDefensiveCopyAndSort(t *testing.T) {
	w := NewMockWorld()
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
	// Mutate the returned slice; confirm a fresh call is unaffected.
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
