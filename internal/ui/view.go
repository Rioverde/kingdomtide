package ui

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/Rioverde/gongeons/internal/game"
	"github.com/Rioverde/gongeons/internal/game/naming"
	pb "github.com/Rioverde/gongeons/internal/proto"
	"github.com/Rioverde/gongeons/internal/ui/locale"
)

// Tint tuning. tintStrengthFactor maps raw influence sum (which can exceed
// 1.0 when several characters overlap at a world coord) to a blend strength
// in [0, 1]; tintCap clamps the blend so the base terrain always remains
// legible even when every influence peaks on the same tile.
const (
	tintStrengthFactor = 0.4
	tintCap            = 0.5

	// voronoiBoundaryHalf is the denominator in the Voronoi boundary-radius
	// formula: the boundary between two Voronoi cells is at the midpoint of
	// the line segment joining their anchors, so the boundary radius from
	// one anchor is exactly half the inter-anchor distance.
	voronoiBoundaryHalf = 2.0

	// resourceBarWidth is the cell count for HP/MP progress bars shown in
	// the stats panel. Six cells keeps the row width identical to the
	// other aligned rows (label + bar + numbers fit on one line inside
	// the stats column without wrapping).
	resourceBarWidth = 6

	// barFilled / barEmpty are the block glyphs shared by every textual
	// progress bar in the UI. The dark-shade/light-shade pair reads as
	// filled/empty in both light and dark terminal themes and stays within
	// the Block Elements range already used elsewhere in the app.
	barFilled = '\u2593' // ▓
	barEmpty  = '\u2591' // ░
)

// progressBar renders a horizontal textual progress bar of exactly width
// cells, proportional to current/max. The bar is saturated (all filled) as
// soon as current >= max so saturated states read as "full" at a glance;
// otherwise the filled count is floor(width * current / max), clamped to
// [0, width]. Returns a string of exactly width runes.
//
// Intended to be reused across future stat bars (HP, mana, stamina, …).
// Callers supply their own width so each bar can be sized independently
// inside its panel.
func progressBar(current, max, width int) string {
	if width <= 0 {
		return ""
	}
	filled := 0
	switch {
	case max <= 0 || current <= 0:
		filled = 0
	case current >= max:
		filled = width
	default:
		filled = (current * width) / max
	}
	var b strings.Builder
	b.Grow(width * 3) // block runes are 3 bytes each in UTF-8
	for i := 0; i < width; i++ {
		if i < filled {
			b.WriteRune(barFilled)
			continue
		}
		b.WriteRune(barEmpty)
	}
	return b.String()
}

// Every glyph this file renders comes from runes.go — runeSelf / runeOther /
// runeUnspecified / riverRune / terrainRunes for the map, plus the chrome
// constants (LogBullet, StatusDivider) for the surrounding UI. User-facing
// text goes through locale.Tr so English and Russian share the same views.

// View renders the full screen for the current phase.
func (m *Model) View() string {
	switch m.phase {
	case phaseEnterName:
		return m.viewEnterName()
	case phaseCharacterCreation:
		return m.viewCharacterCreation()
	case phaseConnecting:
		return m.viewConnecting()
	case phasePlaying:
		return m.viewPlaying()
	case phaseDisconnected:
		return m.viewDisconnected()
	}
	return ""
}

// centeredBox renders inner with the given style, then centres the result
// in the terminal. Falls back to the uncentred box before the first
// WindowSizeMsg so lipgloss.Place is never asked to fit into a 0×0 canvas.
func (m *Model) centeredBox(style lipgloss.Style, inner string) string {
	box := style.Render(inner)
	if m.termWidth <= 0 || m.termHeight <= 0 {
		return box
	}
	return lipgloss.Place(
		m.termWidth, m.termHeight,
		lipgloss.Center, lipgloss.Center,
		box,
	)
}

// viewEnterName draws the sword banner, game title, tagline, name input and
// quit hint — all stacked and centred inside a bordered box, which is itself
// centred on the terminal.
func (m *Model) viewEnterName() string {
	inner := lipgloss.JoinVertical(lipgloss.Center,
		styles.title.Render(TitleArt),
		"",
		styles.title.Render(locale.Tr(m.lang, locale.KeyTitleText)),
		styles.prompt.Render(locale.Tr(m.lang, locale.KeyTitleTagline)),
		"",
		styles.prompt.Render(locale.Tr(m.lang, locale.KeyInputNameLabel)),
		renderInput(m),
		"",
		styles.status.Render(locale.Tr(m.lang, locale.KeyHintQuitLong)),
	)
	return m.centeredBox(styles.box, inner)
}

// viewConnecting shows a centred "connecting" notice with an animated
// spinner so the wait never feels frozen.
func (m *Model) viewConnecting() string {
	connecting := locale.Tr(m.lang, locale.KeyStatusConnecting, "Address", m.serverAddr)
	inner := lipgloss.JoinVertical(lipgloss.Left,
		styles.status.Render(m.spinner.View()+connecting),
		"",
		styles.status.Render(locale.Tr(m.lang, locale.KeyHintQuitShort)),
	)
	return m.centeredBox(styles.box, inner)
}

// viewDisconnected shows the error in a red box plus a quit hint,
// centred in the terminal area.
func (m *Model) viewDisconnected() string {
	msg := locale.Tr(m.lang, locale.KeyStatusDisconnected)
	if m.err != nil {
		msg = locale.Tr(m.lang, locale.KeyStatusDisconnectedWithError,
			"Error", m.err.Error())
	}
	inner := lipgloss.JoinVertical(lipgloss.Left,
		msg,
		"",
		locale.Tr(m.lang, locale.KeyHintDisconnect),
	)
	return m.centeredBox(styles.errBox, inner)
}

// viewPlaying composes the map (with embedded status strip), stats panel,
// and events panel.
//
// Wide layout (termWidth >= minTermWidth):
//
//	┌──────────────────────────────┐    ┌──────────────┐
//	│                              │    │    Stats     │
//	│             Map              │    │              │
//	│                              │    │ (no stats    │
//	│──────────────────────────────│    │  yet)        │
//	│ you: x │ region │ x,y │ ...  │    └──────────────┘
//	└──────────────────────────────┘
//	┌───────────────┐
//	│    Events     │
//	└───────────────┘
//
// The status strip lives inside the map box, separated from the tile grid
// by a horizontal rule. No separate bottom bar; keybindings are removed
// from the status line (discoverable via ? help).
//
// Narrow fallback (termWidth < minTermWidth) keeps a stacked layout so
// the game remains usable on 60-column or smaller terminals.
func (m *Model) viewPlaying() string {
	if m.termWidth < minTermWidth {
		// Narrow single-column layout: map / stats / log.
		grid := m.renderMapBox()
		stats := m.renderStatsBox()
		events := m.renderLog()
		return lipgloss.JoinVertical(lipgloss.Left, grid, stats, events)
	}

	stats := m.renderStatsBox()
	rightCol := lipgloss.JoinVertical(lipgloss.Left, stats)

	mapBox := m.renderMapBox()
	events := m.renderEventsBox()
	leftCol := lipgloss.JoinVertical(lipgloss.Left, mapBox, events)

	// Distribute horizontal slack evenly on both sides of the right column
	// so it appears centred between the map's right edge and the terminal
	// right edge, matching the mockup's equal-padding aesthetic.
	slack := max(0, m.termWidth-lipgloss.Width(leftCol)-sidebarWidth)
	sideGap := max(columnGap, slack/2)
	return lipgloss.JoinHorizontal(lipgloss.Top,
		leftCol, strings.Repeat(" ", sideGap), rightCol)
}

// renderStatsBox renders the right-side character stats panel.
//
// Before join (no coreStats set) it shows the player name and a
// placeholder so the box is never empty. After phaseCharacterCreation
// completes the box renders the full block:
//
//	{name — yellow bold}
//
//	STR  8  -1
//	DEX 10   0
//	CON 14  +2
//	INT 10   0
//	WIS 10   0
//	CHA 10   0
//
//	HP  22/22
//	MP   5
//	SPD  1
//
// Stat labels come from the creation.stat.* locale keys so they are
// already localised (EN: STR/DEX/…, RU: СИЛ/ЛОВ/…). The derived rows
// use stats.hp / stats.mp / stats.speed. Modifiers are right-aligned
// to a fixed 3-char width: "+2", " 0", "-1".
func (m *Model) renderStatsBox() string {
	innerW := sidebarWidth - gridBoxChrome
	if innerW < 4 {
		innerW = 4
	}

	nameStyled := ""
	name := displayName(m.nameInput.Value(), m.myID)
	if name != "" {
		nameStyled = lipgloss.NewStyle().
			Foreground(lipgloss.Color("11")).
			Bold(true).
			Render(name)
	}

	// Before character creation is confirmed, show the placeholder.
	zero := game.CoreStats{}
	if m.coreStats == zero {
		empty := locale.Tr(m.lang, locale.KeyStatsEmpty)
		var content string
		if nameStyled != "" {
			content = nameStyled + "\n" + empty
		} else {
			content = empty
		}
		return styles.playerL.Width(innerW).Render(content)
	}

	// Stat label keys and the matching raw values, in order.
	statKeys := [statsCount]string{
		locale.KeyCreationStatStrength,
		locale.KeyCreationStatDexterity,
		locale.KeyCreationStatConstitution,
		locale.KeyCreationStatIntelligence,
		locale.KeyCreationStatWisdom,
		locale.KeyCreationStatCharisma,
	}
	statVals := [statsCount]int{
		m.coreStats.Strength,
		m.coreStats.Dexterity,
		m.coreStats.Constitution,
		m.coreStats.Intelligence,
		m.coreStats.Wisdom,
		m.coreStats.Charisma,
	}

	var b strings.Builder
	if nameStyled != "" {
		b.WriteString(nameStyled)
		b.WriteByte('\n')
	}
	b.WriteByte('\n') // spacer after name

	for i := range statsCount {
		label := locale.Tr(m.lang, statKeys[i])
		val := statVals[i]
		mod := game.Modifier(val)
		var modStr string
		switch {
		case mod > 0:
			modStr = fmt.Sprintf("+%d", mod)
		case mod < 0:
			modStr = fmt.Sprintf("%d", mod)
		default:
			modStr = " 0"
		}
		// Format: "LBL  VV  MOD" — label left, value and modifier right-padded.
		b.WriteString(fmt.Sprintf("%-3s %2d %3s\n", label, val, modStr))
	}

	b.WriteByte('\n') // spacer before derived stats

	maxHP := m.coreStats.MaxHP()
	mp := m.coreStats.Mana()
	spd := m.coreStats.DerivedSpeed()

	hpLabel := locale.Tr(m.lang, locale.KeyStatsHP)
	mpLabel := locale.Tr(m.lang, locale.KeyStatsMP)
	spdLabel := locale.Tr(m.lang, locale.KeyStatsSpeed)

	// HP gets a visible progress bar so damage is legible at a glance.
	// MP gets one too when the character has any mana at all (low-Int
	// builds with Mana() == 0 skip the row entirely). Derived Speed is
	// a plain number — it's a constant readout, not a resource to drain.
	// Bars are tinted with the conventional roguelike palette: red for
	// hit points, blue for mana.
	hp := styles.hpBar.Render(progressBar(m.currentHP, maxHP, resourceBarWidth))
	b.WriteString(fmt.Sprintf("%-3s %s %d/%d\n", hpLabel, hp, m.currentHP, maxHP))

	if mp > 0 {
		// Mana regeneration/spending is not yet modelled on the wire —
		// current Mana is pinned at the cap, so the bar is always full.
		// The row still renders because it telegraphs the resource exists
		// and primes the UI for when spells land and the server starts
		// tracking currentMana.
		mpTinted := styles.mpBar.Render(progressBar(mp, mp, resourceBarWidth))
		b.WriteString(fmt.Sprintf("%-3s %s %d/%d\n", mpLabel, mpTinted, mp, mp))
	}

	b.WriteString(fmt.Sprintf("%-3s %d", spdLabel, spd))

	return styles.playerL.Width(innerW).Render(b.String())
}

// renderMapBox wraps the tile grid and the three-row in-map status strip
// in one lipgloss border box. The strip sits at the bottom of the box:
// a blank spacer row for breathing room, then two info rows with left
// and right values aligned to the respective edges.
//
//	┌──────────────────────────────────────────┐
//	│  · · · tile grid · · ·                   │
//	│                                          │
//	│ Vinehollow              localhost:50051  │
//	│ X: 7, Y: -30    w/↑ north · s/↓ south …  │
//	└──────────────────────────────────────────┘
func (m *Model) renderMapBox() string {
	grid := m.renderGridContent()
	status := m.renderInMapStatus(lipgloss.Width(grid))
	inner := lipgloss.JoinVertical(lipgloss.Left, grid, status)
	// Tight vertical padding (0 rows top+bottom) so the status strip sits
	// directly under the tile grid and directly above the bottom border,
	// without wasted dark space.
	return styles.box.Padding(0, 2).Render(inner)
}

// renderInMapStatus returns the three-line status strip rendered inside
// the map box. An empty spacer row provides breathing room between the
// grid and the info rows. The two info rows each hold a left-column and
// a right-column value aligned to their respective edges:
//
//	                                                      (empty spacer)
//	Vinehollow                                 localhost:50051
//	X: 7, Y: -30                      w/↑ north · s/↓ south · ...
//
// totalWidth is the grid width in terminal cells; each info row is padded
// so the right value aligns with the map's right edge. Missing fields
// (no region yet, no self-pos yet, etc.) collapse silently.
func (m *Model) renderInMapStatus(totalWidth int) string {
	var regionName, coords, serverAddr, hints string
	if m.region != nil {
		if name := composeName(naming.DomainRegion, m.region.GetName(), m.lang); name != "" {
			regionName = regionHeaderStyle(m.region.GetCharacter()).Render(name)
		}
	}
	if self, ok := m.selfPlayer(); ok {
		coords = fmt.Sprintf("X: %d, Y: %d", self.Pos.X, self.Pos.Y)
	}
	serverAddr = styles.status.Render(m.serverAddr)
	hints = m.help.ShortHelpView(Keys.ShortHelp())

	row1 := padBetween(regionName, serverAddr, totalWidth)
	row2 := padBetween(coords, hints, totalWidth)
	return row1 + "\n" + row2
}

// padBetween returns "left<spaces>right" whose total display width is
// exactly totalWidth. When the combined widths exceed totalWidth the two
// values are joined with a single space — truncation is left to the
// caller since lipgloss string lengths include zero-width ANSI escapes.
func padBetween(left, right string, totalWidth int) string {
	gap := totalWidth - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

// renderEventsBox renders the events panel below the map. Occupies the
// left half of the map's horizontal span; the right half is reserved for
// a future panel (inventory / combat / inspector). Widths are halved on
// the content width so the two bordered boxes align to the map edges.
// Each entry is coloured by its logKind: green for join, grey for leave,
// default for everything else.
func (m *Model) renderEventsBox() string {
	header := locale.Tr(m.lang, locale.KeyPanelEventsHeader)
	empty := locale.Tr(m.lang, locale.KeyPanelEmptyLog)

	// Events occupies the LEFT HALF of the map's horizontal span.
	leftColW := m.termWidth - sidebarWidth - columnGap
	halfW := leftColW / 2
	// Inner width: half column minus box chrome (1 border + 2 padding each side = 6 total).
	innerW := halfW - gridBoxChrome
	if innerW < 10 {
		innerW = 10
	}

	// Visible line budget: panel height minus vertical chrome.
	visibleLines := eventsRows - eventsBoxChromeV
	if visibleLines < 1 {
		visibleLines = 1
	}

	if len(m.logLines) == 0 {
		return styles.log.Width(innerW).Render(header + "\n" + empty)
	}

	// Show the most recent visibleLines entries, newest at the bottom.
	src := m.logLines
	if len(src) > visibleLines {
		src = src[len(src)-visibleLines:]
	}

	wrapStyle := lipgloss.NewStyle().Width(innerW)
	var b strings.Builder
	b.WriteString(header + "\n")
	for _, entry := range src {
		styled := logEntryStyle(entry).Render(wrapStyle.Render(entry.Text))
		b.WriteString(styled)
		b.WriteByte('\n')
	}
	content := strings.TrimRight(b.String(), "\n")
	return styles.log.Width(innerW).Render(content)
}

// renderGridContent turns the tile slice into a styled multi-line string
// without an outer border box. The border is applied by renderMapBox so
// the status strip can share the same outer chrome. World coordinates are
// reconstructed from the viewport origin plus the local (x, y) offset so
// renderTile2w can feed the tint sampler the same coords the server used when
// resolving the tile's region. Each tile is rendered as tileWidth terminal
// cells wide via renderTile2w for correct aspect ratio.
func (m *Model) renderGridContent() string {
	if m.width <= 0 || m.height <= 0 || len(m.tiles) == 0 {
		return locale.Tr(m.lang, locale.KeyPanelEmptyMap)
	}
	var b strings.Builder
	b.Grow(m.width * m.height * (4 * tileWidth))
	for y := range m.height {
		for x := range m.width {
			idx := y*m.width + x
			if idx >= len(m.tiles) {
				b.WriteString(styles.unknownTile.Render(runeUnspecified + " "))
				continue
			}
			worldX := m.origin.X + x
			worldY := m.origin.Y + y
			b.WriteString(m.renderTile2w(m.tiles[idx], worldX, worldY))
		}
		if y < m.height-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// renderTile2w renders one tile as exactly tileWidth (2) terminal cells. The
// pattern is "glyph + space": the glyph stays in the left cell so the map
// reads like classic DF — each game-tile occupies a roughly square visual
// area — while the trailing space carries the same background tint as the
// glyph cell, preventing colour fringing at region boundaries. Style is
// applied to the full two-character string so the background fills both cells.
func (m *Model) renderTile2w(t *pb.Tile, worldX, worldY int) string {
	if t == nil {
		return styles.unknownTile.Render(runeUnspecified + " ")
	}
	if t.GetOccupant() == pb.OccupantKind_OCCUPANT_PLAYER && t.GetEntityId() != "" {
		if t.GetEntityId() == m.myID {
			return styles.selfPlayer.Render(runeSelf + " ")
		}
		return styles.otherPlayer.Render(runeOther + " ")
	}
	if lm := t.GetLandmark(); lm != nil {
		lk := lm.GetKind()
		if lk != pb.LandmarkKind_LANDMARK_KIND_NONE {
			if glyph, ok := landmarkRunes[lk]; ok {
				style := landmarkStyles[lk]
				return style.Render(glyph + " ")
			}
		}
	}
	if s := t.GetStructure(); s != pb.Structure_STRUCTURE_UNSPECIFIED {
		glyph, gOK := structureRunes[s]
		style, sOK := structureStyles[s]
		if gOK && sOK && glyph != "" {
			return style.Render(glyph + " ")
		}
		return styles.unknownTile.Render(runeUnspecified + " ")
	}
	overlays := game.TileOverlay(t.GetOverlays())
	if overlays.Has(game.OverlayLake) {
		return styles.river.Render(lakeRune + " ")
	}
	if overlays.Has(game.OverlayRiver) {
		return styles.river.Render(riverRune + " ")
	}
	r, s := lookTile(t)
	s = m.tintForTile(s, worldX, worldY)
	return s.Render(r + " ")
}

// renderCell picks the rune + style for one tile. Layer precedence:
// occupant > structure (village / castle) > river overlay > terrain. Self vs
// other player is decided by myID. Terrain tiles inside a dominant-character
// region receive a per-tile tint sampled from the local influenceSource, so
// "heart-of-region" tiles read noticeably more coloured than tiles near the
// Voronoi edge.
func (m *Model) renderCell(t *pb.Tile, worldX, worldY int) string {
	if t == nil {
		return styles.unknownTile.Render(runeUnspecified)
	}
	if t.GetOccupant() == pb.OccupantKind_OCCUPANT_PLAYER && t.GetEntityId() != "" {
		if t.GetEntityId() == m.myID {
			return styles.selfPlayer.Render(runeSelf)
		}
		return styles.otherPlayer.Render(runeOther)
	}
	if s := t.GetStructure(); s != pb.Structure_STRUCTURE_UNSPECIFIED {
		glyph, gOK := structureRunes[s]
		style, sOK := structureStyles[s]
		if gOK && sOK && glyph != "" {
			return style.Render(glyph)
		}
		// Unknown / version-skew structure from the server: render the "what is this"
		// marker so the player SEES something is there, rather than silently
		// falling through to the terrain underneath.
		return styles.unknownTile.Render(runeUnspecified)
	}
	overlays := game.TileOverlay(t.GetOverlays())
	// Lake sits above river in the precedence order: a tile where both flags
	// ended up set is visually a lake (a river trace resolved a depression
	// here and marked it as standing water). The "this is standing water"
	// reading is stronger than "a river runs through here" for basin tiles.
	// TODO(Rioverde): introduce styles.lake if the shared styles.river colour
	// ends up reading same-y against the river glyph in live play.
	if overlays.Has(game.OverlayLake) {
		return styles.river.Render(lakeRune)
	}
	if overlays.Has(game.OverlayRiver) {
		return styles.river.Render(riverRune)
	}
	r, s := lookTile(t)
	s = m.tintForTile(s, worldX, worldY)
	return s.Render(r)
}

// tintForTile applies the region-accent tint to base when the tile at
// (worldX, worldY) has a non-Normal dominant character AND the terminal
// colour profile supports tinting. When any of those conditions fail — no
// influence source (pre-join), all-zero influence, grayscale terminal — the
// base style is returned unchanged so the map degrades gracefully.
func (m *Model) tintForTile(base lipgloss.Style, worldX, worldY int) lipgloss.Style {
	if m.influenceSource == nil {
		return base
	}
	infl := m.influenceSource.InfluenceAt(worldX, worldY)
	character := infl.Dominant()
	if character == game.RegionNormal || infl.Sum() <= 0 {
		return base
	}
	accent := regionAccent(pbCharacter(character))
	if accent == "" {
		return base
	}
	_, sc := game.AnchorAt(m.worldSeed, worldX, worldY)
	anchor := game.AnchorOf(m.worldSeed, sc)
	dist := distanceFalloff(worldX, worldY, anchor, m.worldSeed, sc)
	strength := math.Min(float64(infl.Max())*tintStrengthFactor*dist, tintCap)
	return tintedStyle(base, accent, strength)
}

// distanceFalloff returns a multiplier in [0, 1] that scales tint intensity
// by the tile's distance to its region anchor. 1.0 at the anchor itself, 0.0
// at the Voronoi-cell border (the half-distance to the nearest neighbouring
// anchor). When every neighbouring super-chunk returns the same SC (all nine
// candidates collapsed into the same region — possible near map edges or in
// degenerate seeds), we fall back to treating SuperChunkSize as the
// boundary radius so the falloff is still finite instead of NaN.
func distanceFalloff(worldX, worldY int, anchor game.Position, seed int64, own game.SuperChunkCoord) float64 {
	boundary := boundaryRadius(seed, own)
	if boundary <= 0 {
		return 0
	}
	dx := float64(worldX - anchor.X)
	dy := float64(worldY - anchor.Y)
	d := math.Sqrt(dx*dx + dy*dy)
	if d >= boundary {
		return 0
	}
	return 1.0 - d/boundary
}

// boundaryRadius estimates half the distance from own's anchor to the
// nearest foreign anchor among the 8 neighbouring super-chunks. This mirrors
// Voronoi geometry: the border between two cells is the perpendicular
// bisector of the line joining their anchors, so half the anchor-to-anchor
// distance is the worst-case maximum tile-to-anchor distance inside the
// cell. Returns SuperChunkSize (a sensible constant fallback) when every
// neighbour is the same region, which happens for degenerate seeds and near
// the world edge of the 2^31 tile grid.
func boundaryRadius(seed int64, own game.SuperChunkCoord) float64 {
	ownAnchor := game.AnchorOf(seed, own)
	nearest := math.MaxFloat64
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if dx == 0 && dy == 0 {
				continue
			}
			neighbour := game.SuperChunkCoord{X: own.X + dx, Y: own.Y + dy}
			a := game.AnchorOf(seed, neighbour)
			ddx := float64(a.X - ownAnchor.X)
			ddy := float64(a.Y - ownAnchor.Y)
			d := math.Sqrt(ddx*ddx + ddy*ddy)
			if d < nearest {
				nearest = d
			}
		}
	}
	if nearest == math.MaxFloat64 {
		return float64(game.SuperChunkSize)
	}
	return nearest / voronoiBoundaryHalf
}

// pbCharacter is the reverse of regionCharacterKey / mapper.go's pb-to-
// domain switch: it lifts a domain RegionCharacter back to its wire enum
// value so the per-tile tint (computed locally from influenceSource) can
// reuse the same pb-keyed accent palette the region header uses. Unknown
// values fall through to REGION_CHARACTER_NORMAL — the pipeline already
// short-circuits on Normal, so an unmapped value silently disables tint.
func pbCharacter(c game.RegionCharacter) pb.RegionCharacter {
	switch c {
	case game.RegionBlighted:
		return pb.RegionCharacter_REGION_CHARACTER_BLIGHTED
	case game.RegionFey:
		return pb.RegionCharacter_REGION_CHARACTER_FEY
	case game.RegionAncient:
		return pb.RegionCharacter_REGION_CHARACTER_ANCIENT
	case game.RegionSavage:
		return pb.RegionCharacter_REGION_CHARACTER_SAVAGE
	case game.RegionHoly:
		return pb.RegionCharacter_REGION_CHARACTER_HOLY
	case game.RegionWild:
		return pb.RegionCharacter_REGION_CHARACTER_WILD
	}
	return pb.RegionCharacter_REGION_CHARACTER_NORMAL
}

// logEntryStyle returns the lipgloss style to use for a log entry based on
// its kind. Join entries are green, leave entries are grey, everything else
// is unstyled (inherits from the surrounding panel).
func logEntryStyle(e logEntry) lipgloss.Style {
	switch e.Kind {
	case logKindJoin:
		return styles.logJoin
	case logKindLeave:
		return styles.logLeave
	default:
		return styles.logDefault
	}
}

// selfPlayer returns the current player's info from the players map.
// Returns a zero value and false when the player is not yet joined or
// their entity has not arrived in a snapshot.
func (m *Model) selfPlayer() (playerInfo, bool) {
	if m.myID == "" {
		return playerInfo{}, false
	}
	p, ok := m.players[m.myID]
	return p, ok
}

// renderLog draws the rolling event log panel for the narrow fallback layout.
// Empty state gets its own label so the box doesn't collapse to a single
// border-only line. Per-kind colours are applied the same way as in
// renderEventsBox so the narrow layout is consistent with the wide one.
func (m *Model) renderLog() string {
	header := locale.Tr(m.lang, locale.KeyPanelEventsHeader)
	empty := locale.Tr(m.lang, locale.KeyPanelEmptyLog)
	return renderPanel(header, empty, styles.log, m.logLines,
		func(e logEntry) string { return logEntryStyle(e).Render(e.Text) })
}

// renderInput renders the name buffer. Bubbles' textinput.Model draws its
// own cursor and echoes typed runes; we only prepend the styled input
// prompt chevron so the visual affordance from the old hand-rolled widget
// survives.
func renderInput(m *Model) string {
	prompt := locale.Tr(m.lang, locale.KeyInputPrompt)
	return styles.input.Render(prompt) + m.nameInput.View()
}
