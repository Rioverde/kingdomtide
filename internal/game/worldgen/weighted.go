package worldgen

import "math/rand/v2"

// weighted pairs an arbitrary key with its relative probability weight.
// Weights need not be normalised — pickWeighted normalises at draw time
// so callers can express ratios directly.
type weighted[K comparable] struct {
	Kind   K
	Weight float32
}

// pickWeighted draws a key from a weighted distribution. Returns zero when
// weights is empty or all weights are non-positive. The last entry is
// returned when floating-point rounding causes the cumulative sum to fall
// short of r — this is the correct defensive tail.
func pickWeighted[K comparable](rng *rand.Rand, weights []weighted[K], zero K) K {
	if len(weights) == 0 {
		return zero
	}
	var total float32
	for _, w := range weights {
		total += w.Weight
	}
	if total <= 0 {
		return zero
	}
	r := rng.Float32() * total
	var acc float32
	for _, w := range weights {
		acc += w.Weight
		if r < acc {
			return w.Kind
		}
	}
	return weights[len(weights)-1].Kind
}

// characterPrefixCount mirrors the active.*.toml entries under
// "landmark.prefix.<character>.<idx>" and
// "region.prefix.<character>.<idx>". All seven characters carry five
// prefixes; if a future locale shrinks one bucket, the value here must
// drop in lock-step or naming.Generate will produce out-of-range
// PrefixIndex values.
var characterPrefixCount = map[string]int{
	"normal":   5,
	"blighted": 5,
	"fey":      5,
	"ancient":  5,
	"savage":   5,
	"holy":     5,
	"wild":     5,
}
