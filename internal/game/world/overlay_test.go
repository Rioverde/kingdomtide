package world

import (
	"strings"
	"testing"
)

// TestTileOverlayHas exercises the Has contract across edge cases: the
// empty mask, single-flag queries, composite (multi-bit) queries, and
// the subset/superset distinction. The composite-miss row guards the
// "every bit in o must be set" semantic that a naive `t&o != 0` would
// break.
func TestTileOverlayHas(t *testing.T) {
	tests := []struct {
		name string
		tile TileOverlay
		ask  TileOverlay
		want bool
	}{
		{"empty on empty", 0, 0, true},
		{"empty on set", OverlayRiver, 0, true},
		{"single match", OverlayRiver, OverlayRiver, true},
		{"single miss", OverlayRiver, OverlayRoad, false},
		{"composite full", OverlayRiver | OverlayRoad, OverlayRiver | OverlayRoad, true},
		{"composite partial", OverlayRiver, OverlayRiver | OverlayRoad, false},
		{"superset holds subset", OverlayRiver | OverlayRoad, OverlayRiver, true},
		{"disjoint", OverlayRiver, OverlayBridge, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.tile.Has(tc.ask); got != tc.want {
				t.Errorf("TileOverlay(%b).Has(%b) = %v, want %v",
					tc.tile, tc.ask, got, tc.want)
			}
		})
	}
}

// TestTileOverlayString covers the zero value, a single known flag, a
// composite that must render both flag names joined by '|', and an
// unknown high bit that must fall back to the raw bit number.
func TestTileOverlayString(t *testing.T) {
	if got := TileOverlay(0).String(); got != "0" {
		t.Errorf("empty mask String() = %q, want %q", got, "0")
	}

	if got := OverlayRiver.String(); got != "OverlayRiver" {
		t.Errorf("OverlayRiver.String() = %q, want %q", got, "OverlayRiver")
	}

	combined := (OverlayRiver | OverlayRoad).String()
	if !strings.Contains(combined, "OverlayRiver") || !strings.Contains(combined, "OverlayRoad") {
		t.Errorf("composite String() = %q, want both OverlayRiver and OverlayRoad", combined)
	}
	if !strings.Contains(combined, "|") {
		t.Errorf("composite String() = %q, want '|' separator", combined)
	}

	if got := (TileOverlay(1) << 7).String(); got != "bit7" {
		t.Errorf("unknown high bit String() = %q, want %q", got, "bit7")
	}
}
