package server

// lookupOr returns m[k] if present, otherwise the fallback value. The
// generic form lets us share a single dispatch primitive between every
// enum → wire-enum translation in mapper.go without a per-type helper.
func lookupOr[K comparable, V any](m map[K]V, k K, fallback V) V {
	if v, ok := m[k]; ok {
		return v
	}
	return fallback
}
