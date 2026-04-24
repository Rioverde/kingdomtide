package mechanics

import (
	"github.com/Rioverde/gongeons/internal/game/dice"
	"github.com/Rioverde/gongeons/internal/game/polity"
)

// Diffusion pulse band: 0.01–0.03 of population shifted per year.
// With no trade partners yet we model a per-year diffusion pulse
// against the local distribution itself — the majority absorbs a
// small share from the minority each year until a schism or external
// diffusion breaks the trend.
const (
	religionDiffusionMin = 0.01
	religionDiffusionMax = 0.03
)

// Schism four-gate model: secondary share ≥ 0.4, gap to majority ≤
// 0.2, and Innovation ≥ 45. When all three open (and no Inquisition
// decree is active — placeholder, decrees ship later) we rewrite the
// two contesting faiths to a 60/40 split so the secondary gains
// ground and the majority cedes some. No variant-faith type yet, so
// schism manifests as a fragmentation of the majority's dominance
// rather than a new Faith value.
const (
	schismSecondaryThreshold = 0.4
	schismContestDeltaMax    = 0.2
	schismInnovationMin      = 45
	schismSplitMajority      = 0.6
	schismSplitMinority      = 0.4
)

// ApplyReligionDiffusionYear evolves city.Faiths by a small per-year
// diffusion pulse and then checks the schism four-gate model. Without
// neighbor trade data the MVP models self-diffusion only — when trade
// partners land in a later milestone the diffusion source switches
// from "local majority" to "trade-weighted partner majority" without
// changing this function's signature. currentYear is threaded so
// schism events recorded on city.FaithHistory carry an accurate
// timestamp.
func ApplyReligionDiffusionYear(city *polity.City, stream *dice.Stream, currentYear int) {
	if city.Faiths.IsZero() {
		return
	}
	p := computeMajorityAndSecondary(city)

	// D20 [1, 20] → [religionDiffusionMin, religionDiffusionMax] with
	// a uniform map so the pulse is deterministic per stream draw.
	diffusionBand := religionDiffusionMax - religionDiffusionMin
	pulse := religionDiffusionMin +
		float64(stream.D20()-1)/19.0*diffusionBand
	if greatPersonOf(city, polity.GreatPersonPriest) {
		pulse *= float64(priestReligionMultPermille) / 1000.0
	}

	// Majority grows by pulse; remaining faiths shrink proportionally
	// so sum stays at 1.0 after Normalize. The array is indexed by
	// Faith ordinal, so iteration order is deterministic without the
	// AllFaiths bounce.
	const minorityCount = polity.FaithCount - 1
	for _, f := range polity.AllFaiths() {
		if f == p.majority {
			city.Faiths[f] += pulse
			continue
		}
		city.Faiths[f] = max(0, city.Faiths[f]-pulse/float64(minorityCount))
	}
	city.Faiths.Normalize()

	checkSchismWith(city, currentYear, p)
}

// schismCheckParams carries pre-computed majority/secondary state so
// ApplyReligionDiffusionYear and checkSchismWith share one scan.
type schismCheckParams struct {
	majority       polity.Faith
	secondary      polity.Faith
	majorityShare  float64
	secondaryShare float64
}

// computeMajorityAndSecondary finds the top-2 faiths in a single
// pass over the FaithDistribution array. Ties on share break toward
// the lower Faith ordinal (same semantics as FaithDistribution.Majority).
func computeMajorityAndSecondary(city *polity.City) schismCheckParams {
	var top, sec polity.Faith
	topShare, secShare := -1.0, -1.0
	for f := polity.Faith(0); f < polity.FaithCount; f++ {
		v := city.Faiths[f]
		if v > topShare {
			sec, secShare = top, topShare
			top, topShare = f, v
		} else if v > secShare {
			sec, secShare = f, v
		}
	}
	return schismCheckParams{
		majority: top, majorityShare: topShare,
		secondary: sec, secondaryShare: secShare,
	}
}

// checkSchismWith verifies the four-gate schism model using
// pre-computed majority/secondary so no extra scan is needed.
// When all gates open it rewrites the two contesting faiths to a
// 60/40 split and records a SchismEvent on city.FaithHistory.
func checkSchismWith(city *polity.City, currentYear int, p schismCheckParams) {
	if p.secondaryShare < schismSecondaryThreshold {
		return
	}
	if p.majorityShare-p.secondaryShare > schismContestDeltaMax {
		return
	}
	innovationGate := schismInnovationMin
	innovationGate -= techSchismThresholdReduction(city)
	if greatPersonOf(city, polity.GreatPersonPriest) {
		innovationGate += priestSchismThresholdBump
	}
	if int(city.Innovation) < innovationGate {
		return
	}
	// No Inquisition decree to check yet — when decrees land this
	// becomes `if city.HasActiveDecree(DecreeInquisition) { return }`.

	city.Faiths[p.majority] = schismSplitMajority
	city.Faiths[p.secondary] = schismSplitMinority
	city.Faiths.Normalize()

	city.FaithHistory = append(city.FaithHistory, polity.SchismEvent{
		Year:             currentYear,
		OriginalMajority: p.majority,
		NewSecondary:     p.secondary,
	})
}
