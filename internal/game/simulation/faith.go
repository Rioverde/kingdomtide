package simulation

import (
	"fmt"

	"github.com/Rioverde/gongeons/internal/game/polity"
)

// simFaithEmergePct is the minimum share (as a fraction, not percent) a faith
// must reach before a faith-emerged event is emitted. Set above simFaithEpsilon
// to avoid noise on micro-fluctuations; 0.01 = 1%.
const simFaithEmergePct = 0.01

// regionPreferredFaith maps each RegionCharacter to the faith the region's
// affinity dynamics push settlements toward over generations. Derived from
// the dominant entry in worldgen's campFaithByRegion table.
var regionPreferredFaith = [polity.RegionCharacterCount]polity.Faith{
	polity.RegionNormal:   polity.FaithOldGods,
	polity.RegionBlighted: polity.FaithOldGods,
	polity.RegionFey:      polity.FaithGreenSage,
	polity.RegionAncient:  polity.FaithOldGods,
	polity.RegionSavage:   polity.FaithOneOath,
	polity.RegionHoly:     polity.FaithSunCovenant,
	polity.RegionWild:     polity.FaithGreenSage,
}

// blendFaiths returns the population-weighted blend of two distributions.
// Used at merge time so the surviving settlement's faith reflects the
// combined population proportionally.
func blendFaiths(a, b polity.FaithDistribution, popA, popB int) polity.FaithDistribution {
	total := float64(popA + popB)
	if total <= 0 {
		return a
	}
	var result polity.FaithDistribution
	for i := range result {
		result[i] = (float64(popA)*a[i] + float64(popB)*b[i]) / total
	}
	result.Normalize()
	return result
}

// tickFaithConversion advances every live settlement's FaithDistribution by
// one simulated year via the Markov-chain conversion model. Three forces act:
//  1. Conformity: minority faiths bleed adherents toward the majority.
//  2. Dissidence: the majority leaks adherents to minorities.
//  3. Region affinity: all non-preferred faiths lose a fraction to the
//     region's preferred faith, shifting distributions over generations.
//
// After applying conversion, emits faith-emerged when a faith crosses 1% for
// the first time in a settlement, and faith-flipped when the majority changes.
func (s *state) tickFaithConversion(year int) {
	for _, id := range s.sortedSettlementIDs() {
		set := s.settlements[id].Base()

		fdBefore := set.Faiths
		majBefore := majorityIndex(fdBefore)

		applyFaithConversion(&set.Faiths, set.Region)

		majAfter := majorityIndex(set.Faiths)
		if majBefore != majAfter {
			s.log.emit(year, "faith-flipped", fmt.Sprintf(
				"'%s' — majority shifted from %s to %s",
				set.Name, polity.Faith(majBefore), polity.Faith(majAfter)))
		}

		emerged := s.faithEmerged[id]
		for i := 0; i < polity.FaithCount; i++ {
			if !emerged[i] && set.Faiths[i] >= simFaithEmergePct {
				s.log.emit(year, "faith-emerged", fmt.Sprintf(
					"'%s' — %s reached %.2f%%",
					set.Name, polity.Faith(i), set.Faiths[i]*100))
				emerged[i] = true
			}
		}
		s.faithEmerged[id] = emerged
	}
}

// applyFaithConversion applies one year of faith dynamics to a single
// FaithDistribution. Extracted from tickFaithConversion so tests can drive it
// directly without building a full state.
//
// Bookkeeping is symmetric: only above-ε minorities both lose to conformity
// AND receive from dissidence. Below-ε faiths are treated as truly extinct —
// they neither contribute nor receive. The dissidence outflow is split among
// active minorities only, so it does not vanish into the void.
func applyFaithConversion(fd *polity.FaithDistribution, region polity.RegionCharacter) {
	majIdx := majorityIndex(*fd)

	old := *fd

	// Count active minorities (above ε). Used as the dissidence divisor so
	// dissidence outflow is conserved across living faiths only.
	activeMin := 0
	for i := 0; i < polity.FaithCount; i++ {
		if i == majIdx {
			continue
		}
		if old[i] >= simFaithEpsilon {
			activeMin++
		}
	}

	// Conformity: total inbound flow from active minorities to the majority.
	var inbound float64
	for i := 0; i < polity.FaithCount; i++ {
		if i == majIdx {
			continue
		}
		if old[i] < simFaithEpsilon {
			continue
		}
		inbound += old[i] * simFaithConformityRate
	}

	if activeMin == 0 {
		// Single-faith settlement — dissidence has nowhere to go.
		// Skip dissidence redistribution; only majority self-decay applies.
		// Region affinity below still nudges the distribution toward the
		// region's preferred faith from the (effectively zero) minorities.
		fd[majIdx] = old[majIdx] - old[majIdx]*simFaithDissidenceRate
		for i := 0; i < polity.FaithCount; i++ {
			if i == majIdx {
				continue
			}
			fd[i] = old[i]
		}
		applyRegionAffinity(fd, region)
		fd.Normalize()
		return
	}

	// Dissidence: outbound flow from majority split equally across ACTIVE
	// minorities. Below-ε faiths receive nothing — they are truly extinct
	// and dissidence will never re-pump them.
	outbound := old[majIdx] * simFaithDissidenceRate
	perMin := outbound / float64(activeMin)

	// Apply conformity and dissidence.
	fd[majIdx] = old[majIdx] - outbound + inbound
	for i := 0; i < polity.FaithCount; i++ {
		if i == majIdx {
			continue
		}
		if old[i] < simFaithEpsilon {
			// Truly extinct: no conformity loss, no dissidence inflow.
			fd[i] = old[i]
			continue
		}
		loss := old[i] * simFaithConformityRate
		fd[i] = old[i] - loss + perMin
	}

	applyRegionAffinity(fd, region)
	fd.Normalize()
}

// applyRegionAffinity shifts each non-preferred faith's share toward the
// region's preferred faith by simFaithRegionAffinityRate. Reads from the
// CURRENT (post-conformity) state of fd and updates in place — the read
// uses a snapshot of `fd` taken BEFORE this loop so each pass uses a
// consistent base, mirroring conformity/dissidence which both read `old`.
func applyRegionAffinity(fd *polity.FaithDistribution, region polity.RegionCharacter) {
	preferred := int(regionPreferredFaith[region])
	base := *fd
	for i := 0; i < polity.FaithCount; i++ {
		if i == preferred {
			continue
		}
		shift := base[i] * simFaithRegionAffinityRate
		fd[i] -= shift
		fd[preferred] += shift
	}
}

// majorityIndex returns the index of the faith with the highest share.
// Ties break toward the lower index for stable determinism.
func majorityIndex(fd polity.FaithDistribution) int {
	idx := 0
	for i := 1; i < polity.FaithCount; i++ {
		if fd[i] > fd[idx] {
			idx = i
		}
	}
	return idx
}
