package game

import "testing"

func TestFloorDiv(t *testing.T) {
	cases := []struct {
		a, b, want int
	}{
		{0, 16, 0},
		{1, 16, 0},
		{15, 16, 0},
		{16, 16, 1},
		{17, 16, 1},
		{-1, 16, -1}, // Go's /: 0. floorDiv: -1.
		{-16, 16, -1},
		{-17, 16, -2},
		{-32, 16, -2},
	}
	for _, tc := range cases {
		if got := floorDiv(tc.a, tc.b); got != tc.want {
			t.Errorf("floorDiv(%d, %d) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestWorldToChunk(t *testing.T) {
	cases := []struct {
		q, r int
		want ChunkCoord
	}{
		{0, 0, ChunkCoord{0, 0}},
		{15, 15, ChunkCoord{0, 0}},
		{16, 0, ChunkCoord{1, 0}},
		{0, 16, ChunkCoord{0, 1}},
		{-1, -1, ChunkCoord{-1, -1}},
		{-16, -16, ChunkCoord{-1, -1}},
		{-17, -17, ChunkCoord{-2, -2}},
	}
	for _, tc := range cases {
		if got := WorldToChunk(tc.q, tc.r); got != tc.want {
			t.Errorf("WorldToChunk(%d, %d) = %+v, want %+v", tc.q, tc.r, got, tc.want)
		}
	}
}

func TestChunkBounds(t *testing.T) {
	c := ChunkCoord{X: 2, Y: -1}
	minQ, maxQ, minR, maxR := c.Bounds()
	if minQ != 32 || maxQ != 48 || minR != -16 || maxR != 0 {
		t.Errorf("Bounds() = [%d,%d) x [%d,%d), want [32,48) x [-16,0)", minQ, maxQ, minR, maxR)
	}
}

func TestChunkAtRoundTrip(t *testing.T) {
	c := Chunk{Coord: ChunkCoord{X: 3, Y: -2}}
	minQ, _, minR, _ := c.Bounds()
	tile := Tile{Terrain: TerrainJungle}
	c.Set(minQ+5, minR+7, tile)
	if got := c.At(minQ+5, minR+7); got != tile {
		t.Fatalf("At/Set round trip: got %+v, want %+v", got, tile)
	}
}

func TestChunkAtOutOfBoundsPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for out-of-bounds access")
		}
	}()
	c := Chunk{Coord: ChunkCoord{X: 0, Y: 0}}
	_ = c.At(ChunkSize, 0)
}

func TestChunkToSuperChunk(t *testing.T) {
	cases := []struct {
		cc   ChunkCoord
		want SuperChunkCoord
	}{
		{ChunkCoord{0, 0}, SuperChunkCoord{0, 0}},
		{ChunkCoord{3, 3}, SuperChunkCoord{0, 0}},
		{ChunkCoord{4, 0}, SuperChunkCoord{1, 0}},
		{ChunkCoord{0, 4}, SuperChunkCoord{0, 1}},
		{ChunkCoord{-1, -1}, SuperChunkCoord{-1, -1}},
		{ChunkCoord{-4, -4}, SuperChunkCoord{-1, -1}},
		{ChunkCoord{-5, -5}, SuperChunkCoord{-2, -2}},
		{ChunkCoord{7, -5}, SuperChunkCoord{1, -2}},
	}
	for _, tc := range cases {
		if got := ChunkToSuperChunk(tc.cc); got != tc.want {
			t.Errorf("ChunkToSuperChunk(%+v) = %+v, want %+v", tc.cc, got, tc.want)
		}
	}
}

func TestSuperChunkChunkBounds(t *testing.T) {
	cases := []struct {
		sc                             SuperChunkCoord
		wantMinCX, wantMaxCX, wantMinCY, wantMaxCY int
	}{
		{SuperChunkCoord{0, 0}, 0, 4, 0, 4},
		{SuperChunkCoord{1, 0}, 4, 8, 0, 4},
		{SuperChunkCoord{-1, -1}, -4, 0, -4, 0},
		{SuperChunkCoord{2, -2}, 8, 12, -8, -4},
	}
	for _, tc := range cases {
		minCX, maxCX, minCY, maxCY := tc.sc.ChunkBounds()
		if minCX != tc.wantMinCX || maxCX != tc.wantMaxCX || minCY != tc.wantMinCY || maxCY != tc.wantMaxCY {
			t.Errorf("SuperChunkCoord%+v.ChunkBounds() = [%d,%d)×[%d,%d), want [%d,%d)×[%d,%d)",
				tc.sc, minCX, maxCX, minCY, maxCY,
				tc.wantMinCX, tc.wantMaxCX, tc.wantMinCY, tc.wantMaxCY)
		}
	}
}

// TestChunkToSuperChunkRoundTrip verifies that every chunk coord produced by
// SuperChunkCoord.ChunkBounds maps back to that same super-chunk.
func TestChunkToSuperChunkRoundTrip(t *testing.T) {
	for _, sc := range []SuperChunkCoord{{0, 0}, {1, -1}, {-2, 3}} {
		minCX, maxCX, minCY, maxCY := sc.ChunkBounds()
		for cy := minCY; cy < maxCY; cy++ {
			for cx := minCX; cx < maxCX; cx++ {
				got := ChunkToSuperChunk(ChunkCoord{X: cx, Y: cy})
				if got != sc {
					t.Errorf("ChunkToSuperChunk(ChunkCoord{%d,%d}) = %+v, want %+v", cx, cy, got, sc)
				}
			}
		}
	}
}
