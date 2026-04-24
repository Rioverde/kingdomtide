package mechanics

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/polity"
)

// TestApplyTradeYear_InBounds verifies TradeScore never escapes [0, 100]
// across the full population range — catches any regression where the
// sum of weights + scale produces out-of-range values.
func TestApplyTradeYear_InBounds(t *testing.T) {
	cases := []int{0, 80, 500, 5000, 20000, 40000}
	for _, pop := range cases {
		c := polity.City{Settlement: polity.Settlement{Population: pop}}
		ApplyTradeYear(&c)
		if c.TradeScore < 0 || c.TradeScore > 100 {
			t.Errorf("pop=%d: TradeScore=%d out of [0, 100]", pop, c.TradeScore)
		}
	}
}

// TestApplyTradeYear_MonotoneOnPopulation verifies bigger cities get
// better trade scores (with stub inputs held constant). This is the
// property that keeps the economy's growth feedback stable — bigger
// cities produce more trade, which feeds prosperity, which feeds
// growth.
func TestApplyTradeYear_MonotoneOnPopulation(t *testing.T) {
	small := polity.City{Settlement: polity.Settlement{Population: 100}}
	large := polity.City{Settlement: polity.Settlement{Population: 10000}}
	ApplyTradeYear(&small)
	ApplyTradeYear(&large)
	if large.TradeScore <= small.TradeScore {
		t.Errorf("large=%d should exceed small=%d",
			large.TradeScore, small.TradeScore)
	}
}

// TestApplyTradeYear_ZeroPop verifies a population of zero still
// produces a plausible score from the neutral stub fallbacks — the
// function does not crash or return zero for empty settlements,
// because the neighbor / water / deposit placeholders still
// contribute their 0.5-weighted mid-range signal.
func TestApplyTradeYear_ZeroPop(t *testing.T) {
	c := polity.City{}
	ApplyTradeYear(&c)
	if c.TradeScore == 0 {
		t.Errorf("zero-pop city still has stub contribution; got 0")
	}
}
