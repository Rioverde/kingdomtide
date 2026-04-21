package ui

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/Rioverde/gongeons/internal/game"
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
)

// Every glyph this file renders comes from runes.go — runeSelf / runeOther /
// runeUnspecified / riverRune / terrainRunes for the map, plus the chrome
// constants (LogBullet, StatusDivider) for the surrounding UI. User-facing
// text goes through locale.Tr so English and Russian share the same views.

// View renders the full screen for the current phase.
func (m *Model) View() string {
	switch m.phase {
	case phaseEnterName:
		return m.viewEnterName()
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

// renderStatsBox renders the right-side character stats panel. The first
// content line is the player's display name, followed by the stats
// placeholder. Future iterations will list HP, Agility, Intellect, and
// other character parameters once the stats model is wired into the
// snapshot pipeline.
//
//	┌──────────────┐
//	│ Name         │       ← player name
//	│ HP:   12/20  │       ← future stat rows
//	│ AGI:      8  │
//	│ INT:     14  │
//	└──────────────┘
func (m *Model) renderStatsBox() string {
	empty := locale.Tr(m.lang, locale.KeyStatsEmpty)
	innerW := sidebarWidth - 6
	if innerW < 4 {
		innerW = 4
	}
	name := displayName(m.nameInput.Value(), m.myID)
	var content string
	if name != "" {
		nameStyled := lipgloss.NewStyle().
			Foreground(lipgloss.Color("11")).
			Bold(true).
			Render(name)
		content = nameStyled + "\n" + empty
	} else {
		content = empty
	}
	return styles.playerL.Width(innerW).Render(content)
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
	if m.region != nil && m.region.GetName() != "" {
		regionName = regionHeaderStyle(m.region.GetCharacter()).
			Render(m.region.GetName())
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
	innerW := halfW - 6
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
// renderCell can feed the tint sampler the same coords the server used when
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
	// ended up set is visually a lake (the priority-flood pass raised it into
	// a basin), even if flow accumulation also traced a tributary through it.
	// The "this is standing water" reading is stronger than "a river runs
	// through here" for basin tiles.
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
	strength := math.Min(float64(infl.Sum())*tintStrengthFactor*dist, tintCap)
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

// sortedPlayers returns the Model's players ordered by ID.
func (m *Model) sortedPlayers() []playerInfo {
	return sortedMapValues(m.players)
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
