package dice_test

import (
	"strings"
	"testing"

	"github.com/Rioverde/gongeons/internal/game/dice"
)

// TestExitGateErrors pins two parse-rejection scenarios: "1d1!" fails
// on unreachable terminator (reached transitively via the sides>=2
// check), and "4d6d1" fails on the ambiguous-modifier rule with a
// concrete 1-indexed column position.
func TestExitGateErrors(t *testing.T) {
	_, err := dice.Parse("1d1!")
	if err == nil || !strings.Contains(err.Error(), "dice: ") {
		t.Fatalf("1d1! expected dice: error, got %v", err)
	}

	_, err = dice.Parse("4d6d1")
	if err == nil {
		t.Fatal("4d6d1 must return an error")
	}
	pe, ok := err.(*dice.ParseError)
	if !ok {
		t.Fatalf("4d6d1 error is not *ParseError: %T", err)
	}
	if !strings.Contains(pe.Msg, "ambiguous modifier 'd'") {
		t.Fatalf("4d6d1 message = %q, expected 'ambiguous modifier d'", pe.Msg)
	}
	if pe.Column != 4 {
		t.Fatalf("4d6d1 column = %d, expected 4", pe.Column)
	}
}
