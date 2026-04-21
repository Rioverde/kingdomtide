package ui

// Constants that decide how much of the terminal the grid gets vs. the
// side panels and status bar. Tweak here if the box chrome changes.
const (
	// rightPanelWidth is the approximate column count reserved on the right
	// side for the player list + event log (each box has a 1-char border
	// plus 1-char horizontal padding on both sides).
	rightPanelWidth = 26

	// gridDivider is the single-column spacer between the grid box and
	// the right panel.
	gridDivider = 1

	// gridBoxChrome is the total columns the grid's lipgloss box adds
	// around its tile content (border left+right plus horizontal padding).
	gridBoxChrome = 6

	// statusRows is the row budget for the status line below the grid
	// plus a little slack for terminal redraw flicker.
	statusRows = 3

	// gridBoxChromeV is the vertical chrome around the grid box (top and
	// bottom border plus padding).
	gridBoxChromeV = 4

	// minViewportW / minViewportH match server's MinViewportWidth /
	// MinViewportHeight. If the terminal is smaller than this, the server
	// still clamps up, so we don't bother cropping here.
	minViewportW = 11
	minViewportH = 7
)

// viewportForTerm computes the widest × tallest odd-sided grid that fits
// inside the given terminal dimensions, after subtracting the side panel,
// status line, and box chrome. Odd sides keep the player glyph perfectly
// centred under the follow-camera.
func viewportForTerm(termW, termH int) (int, int) {
	w := termW - rightPanelWidth - gridDivider - gridBoxChrome
	h := termH - statusRows - gridBoxChromeV
	if w < minViewportW {
		w = minViewportW
	}
	if h < minViewportH {
		h = minViewportH
	}
	if w%2 == 0 {
		w--
	}
	if h%2 == 0 {
		h--
	}
	return w, h
}
