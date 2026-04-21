package server

import "testing"

func TestLookupOrHit(t *testing.T) {
	m := map[string]int{"a": 1, "b": 2}
	if got := lookupOr(m, "a", 99); got != 1 {
		t.Fatalf("lookupOr hit: want 1, got %d", got)
	}
}

func TestLookupOrMiss(t *testing.T) {
	m := map[string]int{"a": 1}
	if got := lookupOr(m, "missing", 99); got != 99 {
		t.Fatalf("lookupOr miss: want 99 (fallback), got %d", got)
	}
}
