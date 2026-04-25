package voronoi

import (
	"math"
	"math/rand/v2"
	"testing"
)

// TestGenerate_BasicShape sanity-checks Generate's output shape on a
// small map: cell count plausible, CellID sized correctly, every
// cell has at least one neighbour, vertices are inside the map.
func TestGenerate_BasicShape(t *testing.T) {
	const w, h, count = 256, 256, 200
	d := Generate(42, w, h, count, 2, 0)

	if d.W != w || d.H != h {
		t.Fatalf("dimensions: got %dx%d, want %dx%d", d.W, d.H, w, h)
	}
	if got := len(d.CellID); got != w*h {
		t.Fatalf("CellID length: got %d, want %d", got, w*h)
	}
	if len(d.Cells) < count/2 || len(d.Cells) > count*2 {
		t.Fatalf("cell count out of plausible range: got %d, target %d", len(d.Cells), count)
	}
	for i, c := range d.Cells {
		if c.ID != uint16(i) {
			t.Errorf("cell %d: ID mismatch (got %d)", i, c.ID)
		}
		if c.CenterX < 0 || c.CenterX > float64(w) || c.CenterY < 0 || c.CenterY > float64(h) {
			t.Errorf("cell %d: center (%g,%g) outside map", i, c.CenterX, c.CenterY)
		}
		if len(c.Neighbors) == 0 {
			t.Errorf("cell %d: no neighbours", i)
		}
	}
	for i, v := range d.Vertices {
		if v.X < 0 || v.X > float64(w) || v.Y < 0 || v.Y > float64(h) {
			t.Errorf("vertex %d: (%g,%g) outside map", i, v.X, v.Y)
		}
	}
}

// TestRasterizeNearest_Correctness checks that every tile gets the
// site that is actually closest among all sites — not just one in
// the bucket window. Uses a brute-force O(N) ground truth.
func TestRasterizeNearest_Correctness(t *testing.T) {
	const w, h = 64, 64
	rng := rand.New(rand.NewPCG(7, 7))
	sites := placeSeeds(rng, w, h, 30)
	cellID := rasterizeNearest(w, h, sites)

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			fx, fy := float64(x), float64(y)
			best := math.MaxFloat64
			want := uint16(0)
			for i, s := range sites {
				dx := s.X - fx
				dy := s.Y - fy
				d := dx*dx + dy*dy
				if d < best {
					best = d
					want = uint16(i)
				}
			}
			if got := cellID[y*w+x]; got != want {
				t.Fatalf("tile (%d,%d): got cell %d, want %d", x, y, got, want)
			}
		}
	}
}

// TestPlaceSeeds_MinDistance verifies the bucket-accelerated rejection
// sampler honours its minimum-inter-point-distance contract.
func TestPlaceSeeds_MinDistance(t *testing.T) {
	const w, h, count = 512, 512, 400
	rng := rand.New(rand.NewPCG(11, 11))
	sites := placeSeeds(rng, w, h, count)
	if len(sites) == 0 {
		t.Fatal("placeSeeds returned no sites")
	}
	minDist := math.Sqrt(float64(w*h)/float64(count)) * 0.65
	minDist2 := minDist * minDist
	for i := range sites {
		for j := i + 1; j < len(sites); j++ {
			dx := sites[i].X - sites[j].X
			dy := sites[i].Y - sites[j].Y
			if d := dx*dx + dy*dy; d < minDist2 {
				t.Fatalf("sites %d and %d closer than minDist (%g < %g)", i, j, math.Sqrt(d), minDist)
			}
		}
	}
}

// TestNeighborsReciprocal checks the cell-adjacency graph is
// undirected — if A lists B, then B must list A.
func TestNeighborsReciprocal(t *testing.T) {
	d := Generate(123, 256, 256, 150, 2, 0)
	for _, c := range d.Cells {
		for _, n := range c.Neighbors {
			found := false
			for _, back := range d.Cells[n].Neighbors {
				if back == c.ID {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("cell %d lists %d as neighbour but not the reverse", c.ID, n)
			}
		}
	}
}

// TestEdges_CellsAreNeighbours sanity-checks that every edge's
// CellL/CellR pair is in fact adjacent in the cell graph.
func TestEdges_CellsAreNeighbours(t *testing.T) {
	d := Generate(7, 256, 256, 150, 2, 0)
	for ei, e := range d.Edges {
		if e.CellL == e.CellR {
			t.Errorf("edge %d connects cell %d to itself", ei, e.CellL)
		}
		neighbours := d.Cells[e.CellL].Neighbors
		isNeighbour := false
		for _, n := range neighbours {
			if n == e.CellR {
				isNeighbour = true
				break
			}
		}
		if !isNeighbour {
			t.Errorf("edge %d: cells %d and %d are not in each other's neighbour lists",
				ei, e.CellL, e.CellR)
		}
	}
}

// TestTouchesEdge tags border cells correctly.
func TestTouchesEdge(t *testing.T) {
	d := Generate(99, 128, 128, 80, 2, 0)
	// Walk the actual border tiles and confirm each cell ID found
	// there is reported as touching the edge.
	for x := 0; x < d.W; x++ {
		if !d.TouchesEdge(d.CellID[x]) {
			t.Errorf("top border cell %d not flagged", d.CellID[x])
		}
		if !d.TouchesEdge(d.CellID[(d.H-1)*d.W+x]) {
			t.Errorf("bottom border cell %d not flagged", d.CellID[(d.H-1)*d.W+x])
		}
	}
}

// TestGenerate_Determinism — same seed gives identical CellID layout
// across runs; required for reproducible worldgen.
func TestGenerate_Determinism(t *testing.T) {
	a := Generate(2026, 256, 256, 150, 2, 0)
	b := Generate(2026, 256, 256, 150, 2, 0)
	if len(a.CellID) != len(b.CellID) {
		t.Fatalf("CellID lengths differ: %d vs %d", len(a.CellID), len(b.CellID))
	}
	for i := range a.CellID {
		if a.CellID[i] != b.CellID[i] {
			t.Fatalf("CellID[%d] mismatch: %d vs %d", i, a.CellID[i], b.CellID[i])
		}
	}
	if len(a.Vertices) != len(b.Vertices) {
		t.Fatalf("vertex counts differ: %d vs %d", len(a.Vertices), len(b.Vertices))
	}
}

// TestFindCorners_AdjacencySize — every reported corner sits on a
// 2×2 window with at least 3 distinct cells.
func TestFindCorners_AdjacencySize(t *testing.T) {
	d := Generate(33, 128, 128, 80, 2, 0)
	verts, vertCells := findCorners(d.CellID, d.W, d.H)
	if len(verts) != len(vertCells) {
		t.Fatalf("verts/vertCells length mismatch: %d vs %d", len(verts), len(vertCells))
	}
	for i, cells := range vertCells {
		if len(cells) < 3 {
			t.Errorf("corner %d only touches %d cells (want ≥3)", i, len(cells))
		}
	}
}
