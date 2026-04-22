package dice_test

import (
	"strings"
	"testing"

	"github.com/Rioverde/gongeons/internal/game/dice"
)

func TestParseRejects(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		errSub string
		col    int // 0 = don't check
	}{
		{"empty", "", "empty expression", 1},
		{"zero count", "0d6", "dice count must be positive", 1},
		{"invalid count zero prefix ok", "007d6", "", 0}, // 7 is legal
		{"missing d", "5", "expression contains no dice term", 1},
		{"missing sides", "2d", "expected sides specifier", 3},
		{"one-sided", "1d1", "dice sides must be at least 2", 3},
		{"count cap", "100000d6", "exceeds safety cap", 1},
		{"sides cap", "1d100000", "exceeds safety cap", 3},
		{"unknown modifier", "3d6x", "unexpected character 'x'", 4},
		{"ambiguous d", "4d6d1", "ambiguous modifier 'd'", 4},
		{"bare k", "4d6k", "expected 'h' or 'l' after 'k'", 4},
		{"bare kh no count", "4d6kh", "expected count after keep modifier", 6},
		{"kh too many", "4d6kh5", "keep count out of range", 6},
		{"dh too many", "4d6dh5", "drop count out of range", 6},
		{"whitespace inside", "2 d6", "whitespace not allowed", 2},
		{"trailing plus without digits", "3d6+", "expected term or constant after sign", 5},
		{"multiple keep", "4d6kh1kl1", "multiple keep/drop modifiers not allowed", 7},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := dice.Parse(tc.input)
			if tc.errSub == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.errSub)
			}
			if !strings.Contains(err.Error(), tc.errSub) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tc.errSub)
			}
			if tc.col > 0 {
				pe, ok := err.(*dice.ParseError)
				if !ok {
					t.Fatalf("err is not *ParseError: %T", err)
				}
				if pe.Column != tc.col {
					t.Fatalf("column = %d, want %d (err=%q)", pe.Column, tc.col, err.Error())
				}
			}
		})
	}
}

// TestParseRejectsUnreachableExplode validates the parse-time check
// for exploding dice that would never terminate — the explode
// condition is true for every face of the die, so no face terminates
// the chain. Parser rejects these before Execute could loop.
func TestParseRejectsUnreachableExplode(t *testing.T) {
	cases := []string{
		"1d2!<=2",    // both faces <= 2 on d2 → unreachable
		"1d6!>0",     // every face > 0
		"1d6!>=1",    // every face >= 1
		"1d6!<=6",    // every face <= 6
		"1d6!<7",     // every face < 7
	}
	for _, src := range cases {
		t.Run(src, func(t *testing.T) {
			_, err := dice.Parse(src)
			if err == nil {
				t.Fatalf("expected error for %q", src)
			}
			if !strings.Contains(err.Error(), "exploding die has no terminating face") {
				t.Fatalf("unexpected error for %q: %v", src, err)
			}
		})
	}
}

func TestParseExplodeD1Rejected(t *testing.T) {
	// 1d1 is rejected by the sides>=2 rule before the explode check
	// can fire, but the point stands — no valid expression can end
	// with d1!.
	_, err := dice.Parse("1d1!")
	if err == nil {
		t.Fatal("expected error for 1d1!")
	}
	if !strings.Contains(err.Error(), "dice sides must be at least 2") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestParseMultipleKeepDropBeatsRangeCheck pins the error ordering in
// parseKeep/parseDrop. "1d6kh1dh1" must surface "multiple keep/drop"
// (the high-order semantic issue), not "count out of range".
func TestParseMultipleKeepDropBeatsRangeCheck(t *testing.T) {
	cases := []string{
		"1d6kh1dh1",
		"2d6kh1dl1",
		"3d6dh1kh1",
	}
	for _, src := range cases {
		t.Run(src, func(t *testing.T) {
			_, err := dice.Parse(src)
			if err == nil {
				t.Fatalf("expected error for %q", src)
			}
			if !strings.Contains(err.Error(), "multiple keep/drop") {
				t.Fatalf("err = %q, want substring %q", err.Error(), "multiple keep/drop")
			}
		})
	}
}

// TestParseSignWithoutDigitsColumn verifies the error for "1d6+-1"
// anchors at the '-' (column 5) — the failure site — rather than the
// preceding '+' (column 4). Column-accurate errors matter for
// notation-style lints surfaced to players.
func TestParseSignWithoutDigitsColumn(t *testing.T) {
	_, err := dice.Parse("1d6+-1")
	if err == nil {
		t.Fatal("expected error for 1d6+-1")
	}
	pe, ok := err.(*dice.ParseError)
	if !ok {
		t.Fatalf("err is not *ParseError: %T", err)
	}
	if pe.Column != 5 {
		t.Fatalf("column = %d, want 5 (err=%q)", pe.Column, err.Error())
	}
	if !strings.Contains(err.Error(), "unexpected sign character") {
		t.Fatalf("err = %q, want unexpected-sign-character message", err.Error())
	}
}

func TestParseErrorMessagePrefix(t *testing.T) {
	_, err := dice.Parse("1d0")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.HasPrefix(err.Error(), "dice: ") {
		t.Fatalf("error does not start with 'dice: ': %q", err.Error())
	}
}
