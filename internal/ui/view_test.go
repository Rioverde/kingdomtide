package ui

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/Rioverde/gongeons/internal/game"
	"github.com/Rioverde/gongeons/internal/game/naming"
	pb "github.com/Rioverde/gongeons/internal/proto"
)

// playingModel returns a Model wired up for phasePlaying with the given
// terminal dimensions and a small set of log lines and tiles.
func playingModel(termW, termH int) *Model {
	m := New(context.Background(), "localhost:0")
	m.setPhase(phasePlaying)
	m.termWidth = termW
	m.termHeight = termH
	m.width = 11
	m.height = 7
	tiles := make([]*pb.Tile, m.width*m.height)
	for i := range tiles {
		tiles[i] = &pb.Tile{Terrain: pb.Terrain_TERRAIN_PLAINS}
	}
	m.tiles = tiles
	m.logLines = []logEntry{
		{Text: "• You push into Vinehollow.", Kind: logKindDefault},
		{Text: "• баба moved", Kind: logKindDefault},
		{Text: "• баба moved", Kind: logKindDefault},
		{Text: "• баба moved", Kind: logKindDefault},
		{Text: "• баба moved", Kind: logKindDefault},
		{Text: "• You feel the weight of The Grim Thicket.", Kind: logKindDefault},
	}
	return m
}

// TestLayoutWide asserts that at 120×40 the new two-column layout is correct:
// map visible, Stats box present in the right column, Region name appears in
// the in-map status strip, Events panel is under the map (not full-width).
func TestLayoutWide(t *testing.T) {
	t.Parallel()
	m := playingModel(120, 40)

	// Inject a region so the region-name text path is exercised. The
	// structured Name drives composeName on render; the composed output
	// is deterministic in BodySeed.
	m.region = &pb.Region{
		Name: &pb.NameParts{
			Character: "normal",
			SubKind:   "forest",
			Format:    pb.NameFormat_NAME_FORMAT_BODY_ONLY,
			BodySeed:  12345,
		},
		Character: pb.RegionCharacter_REGION_CHARACTER_NORMAL,
	}

	out := m.viewPlaying()

	// Map must be visible — it renders the plains rune.
	if !strings.Contains(out, terrainRunes[pb.Terrain_TERRAIN_PLAINS]) {
		t.Error("wide layout (120x40): map (plains rune) not found in output")
	}

	// Stats panel header must appear in the right column.
	if !strings.Contains(out, "no stats") {
		t.Error("wide layout (120x40): Stats panel header not found in output")
	}

	// Region name must appear as the composeName output.
	wantName := composeName(naming.DomainRegion, m.region.GetName(), m.lang)
	if wantName == "" {
		t.Fatal("composeName returned empty — markov corpus for (en, normal) missing?")
	}
	if !strings.Contains(out, wantName) {
		t.Errorf("wide layout (120x40): composed region name %q not found in output", wantName)
	}

	// Events panel header must appear below the map.
	if !strings.Contains(out, "Events") {
		t.Error("wide layout (120x40): Events panel header not found in output")
	}

	// The half-width events panel wraps long lines, so assert on short
	// substrings that stay intact across the soft-wrap boundary.
	wantFragments := []string{"баба moved", "You feel the weight"}
	for _, want := range wantFragments {
		if !strings.Contains(out, want) {
			t.Errorf("wide layout (120x40): output missing %q", want)
		}
	}

	// Events panel budget: content lines must exceed 3 at this terminal size.
	visibleLines := eventsRows - eventsBoxChromeV
	if visibleLines < 3 {
		t.Errorf("wide layout (120x40): events panel only shows %d lines, want at least 3", visibleLines)
	}
}

// TestLayoutClassic asserts that at 80×24 all panels are present and the
// output does not overflow or panic.
func TestLayoutClassic(t *testing.T) {
	t.Parallel()
	m := playingModel(80, 24)
	out := m.viewPlaying()

	if !strings.Contains(out, terrainRunes[pb.Terrain_TERRAIN_PLAINS]) {
		t.Error("classic layout (80x24): map (plains rune) not found in output")
	}
	if !strings.Contains(out, "баба moved") {
		t.Error("classic layout (80x24): expected at least one log line in output")
	}
	if !strings.Contains(out, "no stats") {
		t.Error("classic layout (80x24): Stats panel header not found in output")
	}
	if !strings.Contains(out, "Events") {
		t.Error("classic layout (80x24): Events panel header not found in output")
	}
}

// TestLayoutNarrowNoPanic verifies that a 60×20 terminal does not panic and
// still produces non-empty output — the narrow fallback path must be safe.
func TestLayoutNarrowNoPanic(t *testing.T) {
	t.Parallel()
	m := playingModel(60, 20)
	out := m.viewPlaying()
	if out == "" {
		t.Error("narrow layout (60x20): got empty output, expected non-empty")
	}
}

// TestLayoutWideNoRegion verifies that when no region is set the in-map status
// strip omits the region name — map and Stats panel must still render correctly.
func TestLayoutWideNoRegion(t *testing.T) {
	t.Parallel()
	m := playingModel(120, 40)
	// region is nil by default in playingModel.
	out := m.viewPlaying()

	// Should still render map and stats.
	if !strings.Contains(out, terrainRunes[pb.Terrain_TERRAIN_PLAINS]) {
		t.Error("wide layout no-region (120x40): map not found in output")
	}
	if !strings.Contains(out, "no stats") {
		t.Error("wide layout no-region (120x40): Stats panel not found in output")
	}
}

// TestRenderCellLayerPrecedence exercises the documented rendering layers:
// occupant > structure (village / castle) > river overlay > terrain. The
// table keeps each case self-contained so a regression in one layer doesn't
// drag the others down with it. Assertions use strings.Contains against the
// raw output to avoid hand-rolling ANSI-escape comparisons.
func TestRenderCellLayerPrecedence(t *testing.T) {
	t.Parallel()

	plainsRune := terrainRunes[pb.Terrain_TERRAIN_PLAINS]
	villageRune := structureRunes[pb.Structure_STRUCTURE_VILLAGE]

	cases := []struct {
		name        string
		myID        string
		tile        *pb.Tile
		mustHave    []string
		mustNotHave []string
	}{
		{
			name:        "plain terrain shows terrain rune, not player",
			tile:        &pb.Tile{Terrain: pb.Terrain_TERRAIN_PLAINS},
			mustHave:    []string{plainsRune},
			mustNotHave: []string{runeSelf, runeOther},
		},
		{
			name:        "river overrides terrain",
			tile:        &pb.Tile{Terrain: pb.Terrain_TERRAIN_PLAINS, Overlays: uint32(game.OverlayRiver)},
			mustHave:    []string{riverRune},
			mustNotHave: []string{plainsRune},
		},
		{
			name: "village shows over plain terrain",
			tile: &pb.Tile{
				Terrain:   pb.Terrain_TERRAIN_PLAINS,
				Structure: pb.Structure_STRUCTURE_VILLAGE,
			},
			mustHave: []string{villageRune},
		},
		{
			name: "village wins over river",
			tile: &pb.Tile{
				Terrain:   pb.Terrain_TERRAIN_PLAINS,
				Overlays:  uint32(game.OverlayRiver),
				Structure: pb.Structure_STRUCTURE_VILLAGE,
			},
			mustHave:    []string{villageRune},
			mustNotHave: []string{riverRune},
		},
		{
			name: "self player wins over village",
			myID: "me",
			tile: &pb.Tile{
				Terrain:   pb.Terrain_TERRAIN_PLAINS,
				Structure: pb.Structure_STRUCTURE_VILLAGE,
				Occupant:  pb.OccupantKind_OCCUPANT_PLAYER,
				EntityId:  "me",
			},
			mustHave:    []string{runeSelf},
			mustNotHave: []string{villageRune, runeOther},
		},
		{
			name: "unknown Structure falls back to unspecified rune",
			tile: &pb.Tile{
				Terrain:   pb.Terrain_TERRAIN_PLAINS,
				Structure: pb.Structure(99),
			},
			mustHave:    []string{runeUnspecified},
			mustNotHave: []string{plainsRune},
		},
		{
			name:     "nil tile renders unspecified rune",
			tile:     nil,
			mustHave: []string{runeUnspecified},
		},
		{
			name: "lake overrides terrain",
			tile: &pb.Tile{
				Terrain:  pb.Terrain_TERRAIN_PLAINS,
				Overlays: uint32(game.OverlayLake),
			},
			mustHave:    []string{lakeRune},
			mustNotHave: []string{plainsRune, riverRune},
		},
		{
			name: "lake wins over river when both set",
			tile: &pb.Tile{
				Terrain:  pb.Terrain_TERRAIN_PLAINS,
				Overlays: uint32(game.OverlayLake | game.OverlayRiver),
			},
			mustHave:    []string{lakeRune},
			mustNotHave: []string{riverRune},
		},
		{
			name: "village wins over lake",
			tile: &pb.Tile{
				Terrain:   pb.Terrain_TERRAIN_PLAINS,
				Overlays:  uint32(game.OverlayLake),
				Structure: pb.Structure_STRUCTURE_VILLAGE,
			},
			mustHave:    []string{villageRune},
			mustNotHave: []string{lakeRune},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := &Model{myID: tc.myID}
			out := m.renderCell(tc.tile, 0, 0)
			for _, want := range tc.mustHave {
				if !strings.Contains(out, want) {
					t.Errorf("output %q missing expected glyph %q", out, want)
				}
			}
			for _, bad := range tc.mustNotHave {
				if strings.Contains(out, bad) {
					t.Errorf("output %q unexpectedly contains glyph %q", out, bad)
				}
			}
		})
	}
}

// TestRenderLandmarkPrecedence verifies the landmark layer sits between player
// and structure in renderTile2w: player wins over landmark, landmark wins over
// structure and river overlay.
func TestRenderLandmarkPrecedence(t *testing.T) {
	t.Parallel()

	towerRune := landmarkRunes[pb.LandmarkKind_LANDMARK_KIND_TOWER]
	shrineRune := landmarkRunes[pb.LandmarkKind_LANDMARK_KIND_SHRINE]
	villageRune := structureRunes[pb.Structure_STRUCTURE_VILLAGE]

	cases := []struct {
		name        string
		myID        string
		tile        *pb.Tile
		mustHave    []string
		mustNotHave []string
	}{
		{
			name: "landmark wins over structure",
			tile: &pb.Tile{
				Terrain:   pb.Terrain_TERRAIN_PLAINS,
				Landmark:  &pb.Landmark{Kind: pb.LandmarkKind_LANDMARK_KIND_TOWER},
				Structure: pb.Structure_STRUCTURE_VILLAGE,
			},
			mustHave:    []string{towerRune},
			mustNotHave: []string{villageRune},
		},
		{
			name: "self player wins over landmark",
			myID: "me",
			tile: &pb.Tile{
				Terrain:  pb.Terrain_TERRAIN_PLAINS,
				Landmark: &pb.Landmark{Kind: pb.LandmarkKind_LANDMARK_KIND_TOWER},
				Occupant: pb.OccupantKind_OCCUPANT_PLAYER,
				EntityId: "me",
			},
			mustHave:    []string{runeSelf},
			mustNotHave: []string{towerRune},
		},
		{
			name: "landmark wins over river overlay",
			tile: &pb.Tile{
				Terrain:  pb.Terrain_TERRAIN_PLAINS,
				Landmark: &pb.Landmark{Kind: pb.LandmarkKind_LANDMARK_KIND_SHRINE},
				Overlays: uint32(game.OverlayRiver),
			},
			mustHave:    []string{shrineRune},
			mustNotHave: []string{riverRune},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := &Model{myID: tc.myID}
			out := m.renderTile2w(tc.tile, 0, 0)
			for _, want := range tc.mustHave {
				if !strings.Contains(out, want) {
					t.Errorf("output %q missing expected glyph %q", out, want)
				}
			}
			for _, bad := range tc.mustNotHave {
				if strings.Contains(out, bad) {
					t.Errorf("output %q unexpectedly contains glyph %q", out, bad)
				}
			}
		})
	}
}

// TestTileRenderIsTwoCells asserts that renderTile2w always returns a string
// whose visible width (as measured by lipgloss, which strips ANSI escapes) is
// exactly tileWidth (2) terminal cells. We check a plain terrain tile and a
// player tile to cover the most common rendering paths.
func TestTileRenderIsTwoCells(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		tile *pb.Tile
		myID string
	}{
		{
			name: "plains terrain",
			tile: &pb.Tile{Terrain: pb.Terrain_TERRAIN_PLAINS},
		},
		{
			name: "river overlay",
			tile: &pb.Tile{Terrain: pb.Terrain_TERRAIN_PLAINS, Overlays: uint32(game.OverlayRiver)},
		},
		{
			name: "lake overlay",
			tile: &pb.Tile{Terrain: pb.Terrain_TERRAIN_PLAINS, Overlays: uint32(game.OverlayLake)},
		},
		{
			name: "village structure",
			tile: &pb.Tile{Terrain: pb.Terrain_TERRAIN_PLAINS, Structure: pb.Structure_STRUCTURE_VILLAGE},
		},
		{
			name: "self player",
			myID: "me",
			tile: &pb.Tile{
				Terrain:  pb.Terrain_TERRAIN_PLAINS,
				Occupant: pb.OccupantKind_OCCUPANT_PLAYER,
				EntityId: "me",
			},
		},
		{
			name: "other player",
			tile: &pb.Tile{
				Terrain:  pb.Terrain_TERRAIN_PLAINS,
				Occupant: pb.OccupantKind_OCCUPANT_PLAYER,
				EntityId: "other",
			},
		},
		{
			name: "nil tile",
			tile: nil,
		},
		{
			name: "landmark tower",
			tile: &pb.Tile{
				Terrain:  pb.Terrain_TERRAIN_PLAINS,
				Landmark: &pb.Landmark{Kind: pb.LandmarkKind_LANDMARK_KIND_TOWER},
			},
		},
		{
			name: "landmark shrine",
			tile: &pb.Tile{
				Terrain:  pb.Terrain_TERRAIN_PLAINS,
				Landmark: &pb.Landmark{Kind: pb.LandmarkKind_LANDMARK_KIND_SHRINE},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := &Model{myID: tc.myID}
			out := m.renderTile2w(tc.tile, 0, 0)
			w := lipgloss.Width(out)
			if w != tileWidth {
				t.Errorf("renderTile2w(%q): lipgloss.Width = %d, want %d; raw=%q",
					tc.name, w, tileWidth, out)
			}
		})
	}
}

// TestViewportFillsLargeTerminal verifies that on a very large terminal
// (200×80) the viewport expands to fill the available space rather than
// getting pinned at a DF-style classic cap. Odd-sided requirement for
// follow-cam centring still holds.
func TestViewportFillsLargeTerminal(t *testing.T) {
	t.Parallel()

	w, h := viewportForTerm(200, 80)

	// Viewport should comfortably exceed what used to be the 39×25 cap —
	// otherwise we're still clamping and the map will not feel "expanded".
	if w < 50 {
		t.Errorf("viewportForTerm(200,80): width %d too small, map not filling terminal", w)
	}
	if h < 40 {
		t.Errorf("viewportForTerm(200,80): height %d too small, map not filling terminal", h)
	}
	if w%2 == 0 {
		t.Errorf("viewportForTerm(200,80): width %d is even, want odd", w)
	}
	if h%2 == 0 {
		t.Errorf("viewportForTerm(200,80): height %d is even, want odd", h)
	}
}
