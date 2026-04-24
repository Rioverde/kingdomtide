package worldgen

// DemoWorld is a throw-away visualisation fixture that feeds the
// worldgen-explorer TUI before the real staged pipeline lands.
//
// The generator is intentionally a no-op right now: every grid comes
// back zero-filled, so the explorer draws deep ocean everywhere. This
// is the clean slate we build the real worldgen on top of, stage by
// stage. Each future pipeline stage replaces a bit of the zero output
// with real data (tectonics fills Elevation, climate fills Temperature,
// precipitation fills Moisture, and so on).
//
// Every field is a flat []float32 laid out row-major so the layout
// matches what future pipeline stages will use. The ContinentPreset
// field is preserved on DemoWorld for API compatibility but unused.
type DemoWorld struct {
	Size       WorldSize
	Continents ContinentPreset
	Seed       int64
	Width      int
	Height     int

	Elevation   []float32
	Temperature []float32
	Moisture    []float32
}

// GenerateDemoWorld returns a world of pure deep ocean. The explorer
// TUI treats this as the bootstrap state; real worldgen arrives when
// we start populating Elevation/Temperature/Moisture from real stages.
//
// The ContinentPreset argument is retained for caller compatibility.
func GenerateDemoWorld(seed int64, size WorldSize, _ ContinentPreset) *DemoWorld {
	w, h := size.Dimensions()
	total := w * h
	return &DemoWorld{
		Size:        size,
		Continents:  ContinentTrinity,
		Seed:        seed,
		Width:       w,
		Height:      h,
		Elevation:   make([]float32, total),
		Temperature: make([]float32, total),
		Moisture:    make([]float32, total),
	}
}
