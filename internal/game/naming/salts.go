package naming

// Per-domain PCG salts. Each value is a nothing-up-my-sleeve number — the
// first sixteen hex digits of the fractional part of the square root of a
// small prime. The three values are fresh: they do not collide with any
// 64-bit constant in internal/game/ or internal/game/worldgen/ (verified
// by TestSaltDistinct).
//
// Sources:
//
//	saltRegion       — fractional hex of sqrt(7)  = 0xa54ff53a5f1d36f1
//	saltLandmarkName — fractional hex of sqrt(11) = 0x510e527fade682d1
//	saltSettlement   — fractional hex of sqrt(13) = 0x9b05688c2b3e6c1f
//
// saltLandmarkName is distinct from worldgen.seedSaltLandmarkRaw (which
// drives landmark placement). Placement and naming must not share entropy
// streams, otherwise name variety would be correlated with placement
// patterns.
//
// Two of the three literals have the high bit set and exceed math.MaxInt64
// in their unsigned form, so they cannot be spelled as untyped int64
// constants. Routing through a uint64 variable via toSaltInt64 lets Go
// perform the conversion at runtime with two's-complement wraparound,
// preserving the full 64-bit pattern. This mirrors the pattern used in
// internal/game/superchunk.go and internal/game/worldgen/region_source.go.
var (
	saltRegion       = toSaltInt64(0xa54ff53a5f1d36f1)
	saltLandmarkName = toSaltInt64(0x510e527fade682d1)
	saltSettlement   = toSaltInt64(0x9b05688c2b3e6c1f)
)

// toSaltInt64 reinterprets a uint64 bit pattern as int64. The function
// call turns constant checking off for the conversion so the full 64-bit
// pattern survives regardless of the high bit.
func toSaltInt64(u uint64) int64 { return int64(u) }

// domainSalt resolves a Domain to its per-domain PCG salt. Unknown domains
// collapse onto saltRegion so the generator never panics; this is defensive
// only — every Domain defined in this package has an explicit entry here
// (enforced by TestDomainSaltCoverage).
func domainSalt(d Domain) int64 {
	switch d {
	case DomainRegion:
		return saltRegion
	case DomainLandmark:
		return saltLandmarkName
	case DomainSettlement:
		return saltSettlement
	}
	return saltRegion
}
