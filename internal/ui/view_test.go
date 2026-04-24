package ui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/Rioverde/gongeons/internal/game/calendar"
	"github.com/Rioverde/gongeons/internal/game/naming"
	"github.com/Rioverde/gongeons/internal/game/stats"
	"github.com/Rioverde/gongeons/internal/game/world"
	pb "github.com/Rioverde/gongeons/internal/proto"
	"github.com/Rioverde/gongeons/internal/ui/tilestyle"
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
	if !strings.Contains(out, tilestyle.GlyphForPB(pb.Terrain_TERRAIN_PLAINS)) {
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

	// Events now live in the right sidebar column (~22-wide inner) so
	// long entries wrap at word boundaries. Assert on short substrings
	// that stay intact on a single wrapped row.
	wantFragments := []string{"баба moved", "You feel"}
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

	if !strings.Contains(out, tilestyle.GlyphForPB(pb.Terrain_TERRAIN_PLAINS)) {
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
	if !strings.Contains(out, tilestyle.GlyphForPB(pb.Terrain_TERRAIN_PLAINS)) {
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

	plainsRune := tilestyle.GlyphForPB(pb.Terrain_TERRAIN_PLAINS)
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
			tile:        &pb.Tile{Terrain: pb.Terrain_TERRAIN_PLAINS, Overlays: uint32(world.OverlayRiver)},
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
				Overlays:  uint32(world.OverlayRiver),
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
				Overlays: uint32(world.OverlayLake),
			},
			mustHave:    []string{lakeRune},
			mustNotHave: []string{plainsRune, riverRune},
		},
		{
			name: "lake wins over river when both set",
			tile: &pb.Tile{
				Terrain:  pb.Terrain_TERRAIN_PLAINS,
				Overlays: uint32(world.OverlayLake | world.OverlayRiver),
			},
			mustHave:    []string{lakeRune},
			mustNotHave: []string{riverRune},
		},
		{
			name: "village wins over lake",
			tile: &pb.Tile{
				Terrain:   pb.Terrain_TERRAIN_PLAINS,
				Overlays:  uint32(world.OverlayLake),
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
				Overlays: uint32(world.OverlayRiver),
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
			tile: &pb.Tile{Terrain: pb.Terrain_TERRAIN_PLAINS, Overlays: uint32(world.OverlayRiver)},
		},
		{
			name: "lake overlay",
			tile: &pb.Tile{Terrain: pb.Terrain_TERRAIN_PLAINS, Overlays: uint32(world.OverlayLake)},
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

// statsModel returns a Model in phasePlaying with a confirmed CoreStats
// built from the given six raw ability scores, mirroring what
// confirmCharacterCreation stores.
func statsModel(str, dex, con, intel, wis, cha int) *Model {
	m := New(context.Background(), "localhost:0")
	m.setPhase(phasePlaying)
	m.termWidth = 120
	m.termHeight = 40
	m.nameInput.SetValue("Aldric")
	cs := stats.CoreStats{
		Strength:     str,
		Dexterity:    dex,
		Constitution: con,
		Intelligence: intel,
		Wisdom:       wis,
		Charisma:     cha,
	}
	m.coreStats = cs
	m.currentHP = cs.MaxHP()
	return m
}

// TestStatsBoxRendersFullBlock verifies that renderStatsBox, when coreStats is
// set, emits all six stat labels and both positive/negative modifier signs.
func TestStatsBoxRendersFullBlock(t *testing.T) {
	t.Parallel()
	// STR 15 → +2, DEX 8 → -1, CON 14 → +2, INT 10 → 0, WIS 8 → -1, CHA 10 → 0.
	// Point Buy cost: 9+0+7+2+0+2 = 20 ≠ 27, so we use a non-validated set here
	// because renderStatsBox only reads coreStats directly — no validation.
	m := statsModel(15, 8, 14, 10, 8, 10)
	out := m.renderStatsBox()

	wantLabels := []string{"STR", "DEX", "CON", "INT", "WIS", "CHA"}
	for _, lbl := range wantLabels {
		if !strings.Contains(out, lbl) {
			t.Errorf("renderStatsBox: stat label %q not found in output", lbl)
		}
	}
	// STR 15 → "+2", DEX 8 → "-1"
	if !strings.Contains(out, "+2") {
		t.Error("renderStatsBox: expected positive modifier '+2' not found")
	}
	if !strings.Contains(out, "-1") {
		t.Error("renderStatsBox: expected negative modifier '-1' not found")
	}
	// Player name must appear.
	if !strings.Contains(out, "Aldric") {
		t.Error("renderStatsBox: player name 'Aldric' not found")
	}
}

// TestStatsBoxHPDerived verifies that CON 14 yields MaxHP = 10 + 2*6 = 22
// and renders as "HP 22/22".
func TestStatsBoxHPDerived(t *testing.T) {
	t.Parallel()
	// All 10s except CON=14: Modifier(14)=+2, MaxHP = 10 + 2*6 = 22.
	m := statsModel(10, 10, 14, 10, 10, 10)
	out := m.renderStatsBox()

	if !strings.Contains(out, "22/22") {
		t.Errorf("renderStatsBox: expected 'HP 22/22' for CON 14, got:\n%s", out)
	}
}

// TestStatsBoxModifierSigns verifies the three sign cases: positive, zero,
// negative are all formatted correctly in renderStatsBox output.
// Uses a model with STR=5 (mod -3), DEX=10 (mod 0), CON=15 (mod +2),
// all others 10 — coreStats set directly so Point Buy validation is skipped.
func TestStatsBoxModifierSigns(t *testing.T) {
	t.Parallel()
	m := New(context.Background(), "localhost:0")
	m.setPhase(phasePlaying)
	m.termWidth = 120
	m.termHeight = 40
	m.coreStats = stats.CoreStats{
		Strength:     5,  // mod -3
		Dexterity:    10, // mod  0
		Constitution: 15, // mod +2
		Intelligence: 10,
		Wisdom:       10,
		Charisma:     10,
	}
	m.currentHP = m.coreStats.MaxHP()

	out := m.renderStatsBox()

	if !strings.Contains(out, "+2") {
		t.Errorf("renderStatsBox: expected '+2' for CON 15, not found in:\n%s", out)
	}
	if !strings.Contains(out, " 0") {
		t.Errorf("renderStatsBox: expected ' 0' for DEX 10, not found in:\n%s", out)
	}
	if !strings.Contains(out, "-3") {
		t.Errorf("renderStatsBox: expected '-3' for STR 5, not found in:\n%s", out)
	}
}

// TestStatsBoxEmptyBeforeJoin verifies that before character creation is
// confirmed (coreStats zero value) the panel shows the placeholder text
// instead of stat rows, preserving backward compat for earlier phases.
func TestStatsBoxEmptyBeforeJoin(t *testing.T) {
	t.Parallel()
	m := New(context.Background(), "localhost:0")
	m.setPhase(phasePlaying)
	m.termWidth = 120
	m.termHeight = 40
	// coreStats is the zero value — no stats confirmed yet.
	out := m.renderStatsBox()

	if !strings.Contains(out, "no stats") {
		t.Error("renderStatsBox before join: expected placeholder '(no stats yet)' not found")
	}
	for _, lbl := range []string{"STR", "DEX", "CON", "INT", "WIS", "CHA"} {
		if strings.Contains(out, lbl) {
			t.Errorf("renderStatsBox before join: stat label %q should not appear before join", lbl)
		}
	}
}

// TestStatsBoxNoEnergyRow verifies that the Energy UI has been retired:
// the stats panel is a pure player-resource readout (HP, MP, SPD) and
// the tick-system's Energy counter is not exposed. Regression guard for
// the design decision to hide Energy — if a label re-appears here the
// decision was reverted by accident.
func TestStatsBoxNoEnergyRow(t *testing.T) {
	t.Parallel()
	m := statsModel(10, 10, 10, 10, 10, 10)
	out := m.renderStatsBox()

	for _, forbidden := range []string{"ENG", "ЭНРГ"} {
		if strings.Contains(out, forbidden) {
			t.Errorf("renderStatsBox: label %q found; Energy must not appear in UI", forbidden)
		}
	}
}

// TestStatsBoxHPBar verifies that the HP row carries a filled progress
// bar proportional to currentHP/maxHP and the numeric "current/max"
// suffix. The bar uses the shared progressBar helper. MaxHP is read
// from the domain formula so the test stays stable if the formula is
// retuned (the test documents the RENDER shape, not the balance).
func TestStatsBoxHPBar(t *testing.T) {
	t.Parallel()
	cs := stats.CoreStats{
		Strength: 10, Dexterity: 10, Constitution: 14,
		Intelligence: 10, Wisdom: 10, Charisma: 10,
	}
	m := statsModel(cs.Strength, cs.Dexterity, cs.Constitution,
		cs.Intelligence, cs.Wisdom, cs.Charisma)
	maxHP := cs.MaxHP()
	m.currentHP = maxHP / 2
	out := m.renderStatsBox()

	bar := styles.hpBar.Render(progressBar(m.currentHP, maxHP, resourceBarWidth))
	want := fmt.Sprintf("HP  %s %d/%d", bar, m.currentHP, maxHP)
	if !strings.Contains(out, want) {
		t.Errorf("HP row missing expected bar+counts.\nwant substring: %q\ngot:\n%s", want, out)
	}
}

// TestStatsBoxMPBar verifies that characters with any Mana get an MP row
// with a visible bar. Current MP is pinned at the cap (spending not
// modelled yet), so the bar always renders saturated. Mana value is
// derived from the domain formula, so the test documents the RENDER
// shape independent of game-balance tuning.
func TestStatsBoxMPBar(t *testing.T) {
	t.Parallel()
	cs := stats.CoreStats{
		Strength: 10, Dexterity: 10, Constitution: 10,
		Intelligence: 14, Wisdom: 10, Charisma: 10,
	}
	m := statsModel(cs.Strength, cs.Dexterity, cs.Constitution,
		cs.Intelligence, cs.Wisdom, cs.Charisma)
	mp := cs.Mana()
	out := m.renderStatsBox()

	bar := styles.mpBar.Render(progressBar(mp, mp, resourceBarWidth))
	want := fmt.Sprintf("MP  %s %d/%d", bar, mp, mp)
	if !strings.Contains(out, want) {
		t.Errorf("MP row missing expected bar+counts.\nwant substring: %q\ngot:\n%s", want, out)
	}
}

// TestProgressBar locks in the shape of the shared bar helper: zero-guard,
// saturation behaviour, and proportional fill. Future stat bars (HP, mana,
// …) will reuse this helper, so the test covers the whole contract rather
// than only the current Energy call site.
func TestProgressBar(t *testing.T) {
	t.Parallel()

	full := string(barFilled)
	empty := string(barEmpty)

	tests := []struct {
		name             string
		current, max, wd int
		want             string
	}{
		{"width zero returns empty", 5, 10, 0, ""},
		{"max zero is empty bar", 3, 0, 4, strings.Repeat(empty, 4)},
		{"negative current is empty bar", -2, 10, 4, strings.Repeat(empty, 4)},
		{"saturated fills fully", 12, 12, 6, strings.Repeat(full, 6)},
		{"over-max still saturates", 99, 12, 6, strings.Repeat(full, 6)},
		{"half fills half", 6, 12, 6, strings.Repeat(full, 3) + strings.Repeat(empty, 3)},
		{"floor rounds down", 5, 12, 6, strings.Repeat(full, 2) + strings.Repeat(empty, 4)},
		{"single cell empty", 0, 12, 1, empty},
		{"single cell full", 12, 12, 1, full},
	}
	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := progressBar(tc.current, tc.max, tc.wd)
			if got != tc.want {
				t.Errorf("progressBar(%d,%d,%d) = %q, want %q",
					tc.current, tc.max, tc.wd, got, tc.want)
			}
		})
	}
}

// setGameTime assigns the Model's cached calendar position directly —
// server-authoritative delivery means tests no longer need a local
// Calendar mirror to Derive from. Used by the date HUD rendering
// tests.
func setGameTime(m *Model, year int32, month calendar.Month, day int32) {
	m.gameTime = calendar.GameTime{
		Year:       year,
		Month:      month,
		DayOfMonth: day,
		Season:     calendar.SeasonOf(month),
	}
}

// TestCalendarDateHUD_Rendering asserts that when the Model carries a
// server-delivered GameTime of 15 October Year 1042, the wide-layout
// view renders the expected English date string inside the map box.
func TestCalendarDateHUD_Rendering(t *testing.T) {
	t.Parallel()
	m := playingModel(120, 40)
	setGameTime(m, 1042, calendar.MonthOctober, 15)

	out := m.viewPlaying()

	want := "15 October, Year 1042"
	if !strings.Contains(out, want) {
		t.Errorf("wide layout: calendar date HUD missing %q in output", want)
	}
}

// TestCalendarDateHUD_NoCalendar_Empty asserts that when the Model has
// no GameTime yet (typical of a legacy server with no calendar wired,
// or the window before the first Snapshot / TimeTick arrives) the HUD
// adds no date content to the output — rendering degrades silently.
func TestCalendarDateHUD_NoCalendar_Empty(t *testing.T) {
	t.Parallel()
	m := playingModel(120, 40)
	// Explicit zero — belt-and-braces against future default changes.
	m.calendarCfg = calendarConfig{}
	m.gameTime = calendar.GameTime{}

	out := m.viewPlaying()

	// The template phrases ", Year " (English) and "года" (Russian)
	// would only appear if the HUD rendered — asserting their absence
	// catches both locales with one check.
	for _, forbidden := range []string{"Year 0", "October", "года"} {
		if strings.Contains(out, forbidden) {
			t.Errorf("calendar date HUD leaked into output without Calendar: found %q", forbidden)
		}
	}
}

// TestCalendarDateHUD_RussianLocale asserts that switching Model.lang
// to "ru" produces the localised genitive-case month name and trailing
// "года" marker.
func TestCalendarDateHUD_RussianLocale(t *testing.T) {
	t.Parallel()
	m := playingModel(120, 40)
	m.lang = "ru"
	setGameTime(m, 1042, calendar.MonthOctober, 15)

	out := m.viewPlaying()

	want := "15 октября, 1042 года"
	if !strings.Contains(out, want) {
		t.Errorf("ru layout: calendar date HUD missing %q in output", want)
	}
}

// TestSeasonStyles_CoversEverySeason asserts that every calendar.Season
// variant has an entry in seasonStyles so the top-bar renderer never
// falls through to the unstyled default on a production-path season.
func TestSeasonStyles_CoversEverySeason(t *testing.T) {
	t.Parallel()
	seasons := []calendar.Season{
		calendar.SeasonWinter,
		calendar.SeasonSpring,
		calendar.SeasonSummer,
		calendar.SeasonAutumn,
	}
	for _, s := range seasons {
		if _, ok := seasonStyles[s]; !ok {
			t.Errorf("seasonStyles missing entry for %s", s.Key())
		}
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
