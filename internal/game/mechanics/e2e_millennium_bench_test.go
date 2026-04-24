package mechanics

import (
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/Rioverde/gongeons/internal/game/dice"
	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/polity"
	"github.com/Rioverde/gongeons/internal/game/worldgen"
	"github.com/Rioverde/gongeons/internal/game/worldgen/chunk"
)

const e2eSeed int64 = 2026

// memDelta captures the difference between two MemStats snapshots.
type memDelta struct {
	allocBytes uint64
	mallocs    uint64
	gcCount    uint32
	heapInuse  uint64
}

// deltaOf computes the delta between before and after MemStats.
func deltaOf(before, after runtime.MemStats) memDelta {
	gcCount := uint32(0)
	if after.NumGC >= before.NumGC {
		gcCount = after.NumGC - before.NumGC
	}
	return memDelta{
		allocBytes: after.TotalAlloc - before.TotalAlloc,
		mallocs:    after.Mallocs - before.Mallocs,
		gcCount:    gcCount,
		heapInuse:  after.HeapInuse,
	}
}

func mbOf(bytes uint64) float64 { return float64(bytes) / (1024 * 1024) }

// BenchmarkE2E_FullPipeline_2000Years measures the full user-facing path:
// world generation → city seeding → 2000-year simulation.
// Run with -benchtime=1x since the workload is too large to repeat.
func BenchmarkE2E_FullPipeline_2000Years(b *testing.B) {
	const (
		superRegionW = 10
		superRegionH = 10
		startYear    = 1300
		simYears     = 2000
		cityCount    = 60
		kingdomCount = 10
		villagesPerCity = 3
	)

	totalStart := time.Now()

	// ------------------------------------------------------------------ //
	// Phase A — World Generation
	// ------------------------------------------------------------------ //
	b.StopTimer()
	runtime.GC()
	var msA0, msA1 runtime.MemStats
	runtime.ReadMemStats(&msA0)
	phaseAStart := time.Now()
	b.StartTimer()

	gen := worldgen.NewWorldGenerator(e2eSeed)
	regSrc := worldgen.NewNoiseRegionSource(e2eSeed, gen)
	lmSrc := worldgen.NewNoiseLandmarkSource(e2eSeed, regSrc, gen)
	volSrc := worldgen.NewNoiseVolcanoSource(e2eSeed, gen, lmSrc)
	_ = worldgen.NewNoiseDepositSource(e2eSeed, gen, lmSrc, volSrc)

	// Warm caches by enumerating all chunks in the 10×10 super-region grid.
	// Each super-region is 64 tiles; each chunk is 16 tiles — so 4 chunks
	// per super-region edge, 40 chunks across 10 super-regions.
	chunksPerSR := geom.SuperChunkSize / chunk.ChunkSize // 4
	totalChunksW := superRegionW * chunksPerSR           // 40
	totalChunksH := superRegionH * chunksPerSR           // 40
	for cy := 0; cy < totalChunksH; cy++ {
		for cx := 0; cx < totalChunksW; cx++ {
			_ = gen.Chunk(chunk.ChunkCoord{X: cx, Y: cy})
		}
	}
	// Warm region + landmark caches for every super-chunk.
	for sy := 0; sy < superRegionH; sy++ {
		for sx := 0; sx < superRegionW; sx++ {
			sc := geom.SuperChunkCoord{X: sx, Y: sy}
			_ = regSrc.RegionAt(sc)
		}
	}

	b.StopTimer()
	phaseAElapsed := time.Since(phaseAStart)
	runtime.ReadMemStats(&msA1)
	dA := deltaOf(msA0, msA1)

	b.ReportMetric(float64(phaseAElapsed.Milliseconds()), "phase_A_world_gen_ms")
	b.ReportMetric(mbOf(dA.allocBytes), "phase_A_world_gen_mb_alloc")

	// ------------------------------------------------------------------ //
	// Phase B — City Seeding
	// ------------------------------------------------------------------ //
	runtime.GC()
	var msB0, msB1 runtime.MemStats
	runtime.ReadMemStats(&msB0)
	phaseBStart := time.Now()
	b.StartTimer()

	// Build 10 kingdoms, 6 cities each, with villages.
	kingdoms := make([]*polity.Kingdom, 0, kingdomCount)
	allCities := make([]*polity.City, 0, cityCount)
	cityMap := make(map[string]*polity.City, cityCount)
	cityStreams := make([]*dice.Stream, 0, cityCount)
	cultures := []polity.Culture{
		polity.CultureFeudal, polity.CultureSteppe, polity.CultureFeudal, polity.CultureSteppe,
		polity.CultureFeudal, polity.CultureSteppe, polity.CultureFeudal, polity.CultureSteppe,
		polity.CultureFeudal, polity.CultureSteppe,
	}

	citiesPerKingdom := cityCount / kingdomCount // 6
	cityRank := 0

	for ki := 0; ki < kingdomCount; ki++ {
		founderStream := dice.New(e2eSeed^int64(ki+1)*0x1111, dice.SaltKingdomYear)
		founder := polity.NewRuler(founderStream, startYear-40)
		seedStream := dice.New(e2eSeed^int64(ki+1)*0x2222, dice.SaltKingdomYear)

		var kCities []*polity.City
		for ci := 0; ci < citiesPerKingdom; ci++ {
			name := fmt.Sprintf("K%d-C%d", ki, ci)
			pos := geom.Position{X: ki*10 + ci, Y: ki}
			cityStream := dice.New(e2eSeed^int64(cityRank+1)*0xabc, dice.SaltKingdomYear)

			age := SeedAge(seedStream)
			pop := SeedPopulationZipf(seedStream, cityRank)
			wealth := SeedWealth(seedStream, pop)

			c := polity.NewCity(name, pos, startYear-age, founder)
			c.Population = pop
			c.Wealth = wealth
			c.TaxRate = polity.TaxNormal
			c.Happiness = 60
			c.Culture = cultures[ki]
			c.Deposits = []polity.Deposit{
				{Kind: polity.DepositGold, RemainingYield: 0.8},
				{Kind: polity.DepositIron, RemainingYield: 0.8},
			}

			kCities = append(kCities, c)
			allCities = append(allCities, c)
			cityMap[c.Name] = c
			cityStreams = append(cityStreams, cityStream)
			cityRank++
		}

		k := polity.NewKingdom(
			fmt.Sprintf("K%d", ki),
			fmt.Sprintf("Kingdom %d", ki),
			founder,
			kCities[0].Name,
			polity.SuccessionPrimogeniture,
			startYear-80,
		)
		k.Culture = cultures[ki]
		for _, c := range kCities[1:] {
			k.CityIDs = append(k.CityIDs, c.Name)
		}
		kingdoms = append(kingdoms, k)
	}

	// Villages: villagesPerCity per city.
	villages := make([]*polity.Village, 0, cityCount*villagesPerCity)
	villageStreams := make([]*dice.Stream, 0, cityCount*villagesPerCity)
	for vi, c := range allCities {
		for j := 0; j < villagesPerCity; j++ {
			vname := fmt.Sprintf("%s-v%d", c.Name, j)
			v := polity.NewVillage(vname, geom.Position{}, startYear-50, c.Name)
			v.Population = 80 + j*20
			villages = append(villages, v)
			villageStreams = append(villageStreams, dice.New(e2eSeed^int64(vi*villagesPerCity+j+1)*0xdef, dice.SaltKingdomYear))
		}
	}

	// Kingdom / league / inter-polity streams.
	kStreams := make([]*dice.Stream, kingdomCount)
	for ki := range kingdoms {
		kStreams[ki] = dice.New(e2eSeed^int64(ki+1)*0x3333, dice.SaltKingdomYear)
	}
	interStream := dice.New(e2eSeed^0x5555, dice.SaltKingdomYear)
	mulkStream := dice.New(e2eSeed^0x6666, dice.SaltKingdomYear)

	// Founding league from first two cities.
	league := polity.NewLeague("L1", "Grand Merchant League",
		allCities[0].Name, allCities[1].Name, startYear-20)
	for i := 2; i < min(8, len(allCities)); i++ {
		league.MemberCityIDs = append(league.MemberCityIDs, allCities[i].Name)
	}
	leagueStream := dice.New(e2eSeed^0x7777, dice.SaltKingdomYear)

	b.StopTimer()
	phaseBElapsed := time.Since(phaseBStart)
	runtime.ReadMemStats(&msB1)
	dB := deltaOf(msB0, msB1)

	b.ReportMetric(float64(phaseBElapsed.Milliseconds()), "phase_B_city_seed_ms")
	b.ReportMetric(mbOf(dB.allocBytes), "phase_B_city_seed_mb_alloc")

	// ------------------------------------------------------------------ //
	// Phase C — 2000-year simulation with 500-year checkpoints
	// ------------------------------------------------------------------ //
	runtime.GC()
	var msC0, msC1 runtime.MemStats
	runtime.ReadMemStats(&msC0)
	phaseCStart := time.Now()
	b.StartTimer()

	type checkpoint struct {
		year      int
		elapsed   time.Duration
		heapInuse float64
		gcCount   uint32
	}
	checkpoints := make([]checkpoint, 0, 4)
	var msChk runtime.MemStats
	runtime.ReadMemStats(&msChk)
	prevGC := msChk.NumGC

	for year := startYear; year < startYear+simYears; year++ {
		for i, v := range villages {
			ApplyVillageYear(v, villageStreams[i])
		}
		ResolveVillageToCity(villages, cityMap)

		TickCitiesYear(allCities, cityStreams, year)

		for ki, k := range kingdoms {
			TickKingdomYear(k, cityMap, kStreams[ki], year)
		}

		TickLeagueYear(league, cityMap, leagueStream, year)

		if year%5 == 0 {
			for ki, k := range kingdoms {
				neighbors := make([]*polity.Kingdom, 0, kingdomCount-1)
				for ni, n := range kingdoms {
					if ni != ki {
						neighbors = append(neighbors, n)
					}
				}
				ApplyInterPolityEventsYear(InterPolityContext{
					Origin:    k,
					Neighbors: neighbors,
					Cities:    cityMap,
					Stream:    interStream,
					Year:      year,
				})
			}
		}

		for _, k := range kingdoms {
			ApplyMulkCycleYear(k, cityMap, mulkStream)
		}

		// Checkpoint every 500 years.
		simYear := year - startYear + 1
		if simYear%500 == 0 {
			b.StopTimer()
			var msNow runtime.MemStats
			runtime.ReadMemStats(&msNow)
			checkpoints = append(checkpoints, checkpoint{
				year:      year,
				elapsed:   time.Since(phaseCStart),
				heapInuse: mbOf(msNow.HeapInuse),
				gcCount:   msNow.NumGC - prevGC,
			})
			prevGC = msNow.NumGC
			b.StartTimer()
		}
	}

	b.StopTimer()
	phaseCElapsed := time.Since(phaseCStart)
	runtime.ReadMemStats(&msC1)
	dC := deltaOf(msC0, msC1)

	b.ReportMetric(phaseCElapsed.Seconds(), "phase_C_sim_2000yr_s")
	b.ReportMetric(mbOf(dC.allocBytes), "phase_C_sim_2000yr_mb_alloc")
	b.ReportMetric(float64(dC.gcCount), "phase_C_sim_gc_count")
	b.ReportMetric(mbOf(msC1.HeapInuse), "phase_C_sim_heap_inuse_final_mb")

	totalElapsed := time.Since(totalStart)
	totalAlloc := dA.allocBytes + dB.allocBytes + dC.allocBytes
	b.ReportMetric(totalElapsed.Seconds(), "total_pipeline_s")
	b.ReportMetric(mbOf(totalAlloc), "total_bytes_allocated_mb")

	// Summary table via b.Logf (visible with -v or -bench output).
	b.Logf("\n=== E2E Millennium Benchmark — Seed %d ===", e2eSeed)
	b.Logf("World:  %d×%d super-regions (%d chunks), seed=%d",
		superRegionW, superRegionH, totalChunksW*totalChunksH, e2eSeed)
	b.Logf("Cities: %d across %d kingdoms, %d villages, sim=%d years",
		len(allCities), len(kingdoms), len(villages), simYears)
	b.Logf("")
	b.Logf("%-30s %12s %14s %16s %8s",
		"Phase", "Wall Time", "Alloc (MB)", "Heap Inuse (MB)", "GCs")
	b.Logf("%-30s %12s %14s %16s %8s",
		"-----", "---------", "----------", "---------------", "---")
	b.Logf("%-30s %12s %14.2f %16.2f %8d",
		"A — World Generation",
		phaseAElapsed.Round(time.Millisecond),
		mbOf(dA.allocBytes),
		mbOf(msA1.HeapInuse),
		dA.gcCount,
	)
	b.Logf("%-30s %12s %14.2f %16.2f %8d",
		"B — City Seeding",
		phaseBElapsed.Round(time.Millisecond),
		mbOf(dB.allocBytes),
		mbOf(msB1.HeapInuse),
		dB.gcCount,
	)
	b.Logf("%-30s %12s %14.2f %16.2f %8d",
		"C — 2000-Year Simulation",
		phaseCElapsed.Round(time.Millisecond),
		mbOf(dC.allocBytes),
		mbOf(msC1.HeapInuse),
		dC.gcCount,
	)
	b.Logf("%-30s %12s %14.2f %16s %8s",
		"TOTAL",
		totalElapsed.Round(time.Millisecond),
		mbOf(totalAlloc),
		"—", "—",
	)
	b.Logf("")
	b.Logf("--- Phase C 500-year checkpoints ---")
	b.Logf("%-8s %12s %16s %8s", "Year", "Elapsed", "Heap Inuse (MB)", "GCs")
	for _, cp := range checkpoints {
		b.Logf("%-8d %12s %16.2f %8d",
			cp.year, cp.elapsed.Round(time.Millisecond), cp.heapInuse, cp.gcCount)
	}
}
