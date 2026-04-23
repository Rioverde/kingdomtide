package rivers

// IsValidHeadForTest exports the unexported isValidHead method for use in
// external tests. The function is a pure function of (seed, hx, hy, terrain),
// so it can be tested against any TerrainSampler.
func IsValidHeadForTest(r *NoiseRiverSource, hx, hy int) bool {
	return r.isValidHead(hx, hy)
}
