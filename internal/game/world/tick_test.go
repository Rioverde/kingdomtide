package game

import (
	"errors"
	"testing"
)

// tickTestSource is a minimal TileSource that reports every tile as
// passable plains. Tick tests care about player bookkeeping, not terrain
// — the shared source keeps cases terse.
type tickTestSource struct{}

func (tickTestSource) TileAt(x, y int) Tile {
	_, _ = x, y
	return Tile{Terrain: TerrainPlains}
}

// wallAtSource treats the given tile as an impassable mountain and
// every other tile as passable plains. Used by refund-on-fail tests to
// set up a guaranteed blocked destination without reshaping the
// fixture for every case.
type wallAtSource struct {
	wall Position
}

func (s wallAtSource) TileAt(x, y int) Tile {
	if x == s.wall.X && y == s.wall.Y {
		return Tile{Terrain: TerrainMountain}
	}
	return Tile{Terrain: TerrainPlains}
}

func newTickTestWorld(t *testing.T) *World {
	t.Helper()
	return NewWorld(tickTestSource{})
}

func joinPlayer(t *testing.T, w *World) *Player {
	t.Helper()
	const (
		id   = "p1"
		name = "Alice"
	)
	if _, err := w.ApplyCommand(JoinCmd{PlayerID: id, Name: name}); err != nil {
		t.Fatalf("join %s: %v", id, err)
	}
	p, ok := w.PlayerByID(id)
	if !ok {
		t.Fatalf("player %s missing after join", id)
	}
	return p
}

func TestNewPlayerJoinDefaults(t *testing.T) {
	w := newTickTestWorld(t)
	p := joinPlayer(t, w)

	if p.Speed != SpeedNormal {
		t.Errorf("Speed = %d, want %d", p.Speed, SpeedNormal)
	}
	if p.Energy != BaseActionCost {
		t.Errorf("Energy = %d, want %d", p.Energy, BaseActionCost)
	}
	if p.Initiative != 0 {
		t.Errorf("Initiative = %d, want 0", p.Initiative)
	}
	if p.Intent != nil {
		t.Errorf("Intent = %v, want nil", p.Intent)
	}
}

func TestEnqueueIntentSaves(t *testing.T) {
	w := newTickTestWorld(t)
	p := joinPlayer(t, w)

	want := MoveIntent{DX: 1, DY: 0}
	if err := w.EnqueueIntent("p1", want); err != nil {
		t.Fatalf("EnqueueIntent: %v", err)
	}
	got, ok := p.Intent.(MoveIntent)
	if !ok {
		t.Fatalf("Intent concrete type = %T, want MoveIntent", p.Intent)
	}
	if got != want {
		t.Errorf("Intent = %+v, want %+v", got, want)
	}
}

func TestEnqueueIntentReplaces(t *testing.T) {
	w := newTickTestWorld(t)
	p := joinPlayer(t, w)

	first := MoveIntent{DX: 1, DY: 0}
	second := MoveIntent{DX: 0, DY: -1}
	if err := w.EnqueueIntent("p1", first); err != nil {
		t.Fatalf("first EnqueueIntent: %v", err)
	}
	if err := w.EnqueueIntent("p1", second); err != nil {
		t.Fatalf("second EnqueueIntent: %v", err)
	}

	got, ok := p.Intent.(MoveIntent)
	if !ok {
		t.Fatalf("Intent concrete type = %T, want MoveIntent", p.Intent)
	}
	if got != second {
		t.Errorf("Intent = %+v, want %+v (second replaces first)", got, second)
	}
}

func TestEnqueueIntentUnknownPlayer(t *testing.T) {
	w := newTickTestWorld(t)

	err := w.EnqueueIntent("ghost", MoveIntent{DX: 1})
	if !errors.Is(err, ErrPlayerNotFound) {
		t.Fatalf("err = %v, want ErrPlayerNotFound", err)
	}
}

// M1's Tick stub test has been superseded by TestTickResolvesPendingMoveIntent
// and friends below, which cover the real tick-resolution contract.

func TestMoveIntentCost(t *testing.T) {
	if got := (MoveIntent{DX: 1}).Cost(); got != BaseActionCost {
		t.Errorf("MoveIntent.Cost() = %d, want %d", got, BaseActionCost)
	}
}

// joinPlayerAt joins a player with the given id and name onto the world.
// Unlike joinPlayer it lets tests control the id so ordering assertions
// can depend on the id-ascending tiebreaker.
func joinPlayerAt(t *testing.T, w *World, id, name string) *Player {
	t.Helper()
	if _, err := w.ApplyCommand(JoinCmd{PlayerID: id, Name: name}); err != nil {
		t.Fatalf("join %s: %v", id, err)
	}
	p, ok := w.PlayerByID(id)
	if !ok {
		t.Fatalf("player %s missing after join", id)
	}
	return p
}

// firstMoved returns the first EntityMovedEvent in events. Fails the test
// when none is present; keeps the assertion sites readable.
func firstMoved(t *testing.T, events []Event) EntityMovedEvent {
	t.Helper()
	for _, ev := range events {
		if em, ok := ev.(EntityMovedEvent); ok {
			return em
		}
	}
	t.Fatalf("no EntityMovedEvent in %d events", len(events))
	return EntityMovedEvent{}
}

func TestTickResolvesPendingMoveIntent(t *testing.T) {
	w := newTickTestWorld(t)
	p := joinPlayer(t, w)
	// Default-post-join: Speed=12, Energy=12 (BaseActionCost).
	if err := w.EnqueueIntent("p1", MoveIntent{DX: 1, DY: 0}); err != nil {
		t.Fatalf("EnqueueIntent: %v", err)
	}

	events := w.Tick()

	em := firstMoved(t, events)
	if em.EntityID != "p1" {
		t.Errorf("moved.EntityID = %q, want p1", em.EntityID)
	}
	if p.Intent != nil {
		t.Errorf("Intent = %v, want nil after resolution", p.Intent)
	}
	// Accumulation is clamped at BaseActionCost on every tick, so the
	// post-resolution Energy is always zero for Speed >= Cost entities:
	// min(12 + 12, 12) = 12, then 12 - 12 = 0.
	if p.Energy != 0 {
		t.Errorf("Energy = %d, want 0 (clamped accumulation then full Cost deduction)",
			p.Energy)
	}
}

// TestTickSkipsEntitiesWithoutIntent checks that a tick on an entity with
// no pending Intent emits no events and leaves the Intent slot empty. The
// idle clamp caps Energy at BaseActionCost so a long idle session cannot
// produce displayed values like "1056/12" — any value >= cost rounds to
// exactly cost and the UI bar simply shows saturated.
func TestTickSkipsEntitiesWithoutIntent(t *testing.T) {
	w := newTickTestWorld(t)
	p := joinPlayer(t, w)

	events := w.Tick()

	if len(events) != 0 {
		t.Errorf("events = %d, want 0 (no intent pending)", len(events))
	}
	if p.Intent != nil {
		t.Errorf("Intent = %v, want nil", p.Intent)
	}
	if p.Energy != BaseActionCost {
		t.Errorf("Energy = %d, want %d (idle clamp to BaseActionCost)",
			p.Energy, BaseActionCost)
	}
}

// TestTickIdleClampHoldsSteady exercises the clamp across many idle ticks:
// Energy must reach BaseActionCost and stay pinned there regardless of
// Speed. Without the clamp, Energy would grow by Speed every tick forever.
func TestTickIdleClampHoldsSteady(t *testing.T) {
	w := newTickTestWorld(t)
	p := joinPlayer(t, w)
	p.Energy = 0
	p.Speed = SpeedFast // larger than BaseActionCost so the clamp is observable

	for range 20 {
		_ = w.Tick()
	}
	if p.Energy != BaseActionCost {
		t.Errorf("Energy after 20 idle ticks = %d, want %d (idle clamp)",
			p.Energy, BaseActionCost)
	}
	if p.Intent != nil {
		t.Errorf("Intent = %v, want nil", p.Intent)
	}
}

// TestTickAccumulatesEnergyBelowCost verifies that a drained entity fires its
// intent on the first tick where energy meets cost. Speed is SpeedNormal (12),
// which equals BaseActionCost — mcalcMove has zero leftover so the gain is
// fully deterministic and test results are not seed-dependent. Starting at
// Energy=0 the accumulation in one tick is exactly BaseActionCost, which
// satisfies the cost check and fires the intent.
func TestTickAccumulatesEnergyBelowCost(t *testing.T) {
	w := newTickTestWorld(t)
	p := joinPlayer(t, w)
	// SpeedNormal % BaseActionCost == 0: mcalcMove is deterministic.
	p.Speed = SpeedNormal
	p.Energy = 0
	if err := w.EnqueueIntent("p1", MoveIntent{DX: 1, DY: 0}); err != nil {
		t.Fatalf("EnqueueIntent: %v", err)
	}

	events := w.Tick()
	// min(0 + 12, 12) = 12 >= cost(12): fires on the first tick.
	if len(events) != 1 {
		t.Errorf("tick 1 events = %d, want 1 (energy reached cost)", len(events))
	}
	_ = firstMoved(t, events)
	// min(0+12,12)=12, then 12-12=0.
	if p.Energy != 0 {
		t.Errorf("Energy = %d, want 0 (clamp then Cost deducted)", p.Energy)
	}
	if p.Intent != nil {
		t.Errorf("Intent = %v, want nil (consumed)", p.Intent)
	}
}

func TestTickRefundsOnFail(t *testing.T) {
	// Walled tile one step east of origin. Alice spawns at (0,0) and the
	// intent targets the wall — the destination is blocked.
	w := NewWorld(wallAtSource{wall: Position{X: 1, Y: 0}})
	p := joinPlayerAt(t, w, "p1", "Alice")
	from, _ := w.PositionOf("p1")
	if err := w.EnqueueIntent("p1", MoveIntent{DX: 1, DY: 0}); err != nil {
		t.Fatalf("EnqueueIntent: %v", err)
	}

	events := w.Tick()

	if len(events) != 1 {
		t.Fatalf("events = %d, want 1 IntentFailedEvent", len(events))
	}
	failed, ok := events[0].(IntentFailedEvent)
	if !ok {
		t.Fatalf("event = %T, want IntentFailedEvent", events[0])
	}
	if failed.EntityID != "p1" {
		t.Errorf("failed.EntityID = %q, want p1", failed.EntityID)
	}
	if failed.Reason != ReasonIntentMoveBlocked {
		t.Errorf("failed.Reason = %q, want %q",
			failed.Reason, ReasonIntentMoveBlocked)
	}
	// Refund: cost is NOT deducted, but accumulation still clamps at
	// BaseActionCost — so the post-fail Energy is the clamp ceiling, not
	// startEnergy+Speed (which would exceed the cap).
	if p.Energy != BaseActionCost {
		t.Errorf("Energy = %d, want %d (clamped, no Cost deduction on fail)",
			p.Energy, BaseActionCost)
	}
	if p.Intent != nil {
		t.Errorf("Intent = %v, want nil (consumed)", p.Intent)
	}
	// Position untouched.
	if pos, _ := w.PositionOf("p1"); pos != from {
		t.Errorf("Position = %+v, want %+v (no move on fail)", pos, from)
	}
}

func TestTickDeterministicOrder(t *testing.T) {
	w := newTickTestWorld(t)
	slow := joinPlayerAt(t, w, "aaaa", "Slow")
	fast := joinPlayerAt(t, w, "bbbb", "Fast")
	slow.Speed = SpeedNormal
	fast.Speed = SpeedVeryFast
	slow.Energy = BaseActionCost
	fast.Energy = BaseActionCost
	if err := w.EnqueueIntent("aaaa", MoveIntent{DX: 0, DY: 1}); err != nil {
		t.Fatalf("enqueue slow: %v", err)
	}
	if err := w.EnqueueIntent("bbbb", MoveIntent{DX: 0, DY: 1}); err != nil {
		t.Fatalf("enqueue fast: %v", err)
	}

	events := w.Tick()

	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}
	firstID := events[0].(EntityMovedEvent).EntityID
	if firstID != "bbbb" {
		t.Errorf("first event EntityID = %q, want bbbb (Speed desc)", firstID)
	}
	secondID := events[1].(EntityMovedEvent).EntityID
	if secondID != "aaaa" {
		t.Errorf("second event EntityID = %q, want aaaa", secondID)
	}
}

func TestTickInitiativeTiebreaker(t *testing.T) {
	w := newTickTestWorld(t)
	lowInit := joinPlayerAt(t, w, "aaaa", "LowInit")
	highInit := joinPlayerAt(t, w, "bbbb", "HighInit")
	lowInit.Initiative = 3
	highInit.Initiative = 5
	// Equal Speed — only Initiative separates them.
	lowInit.Speed = SpeedNormal
	highInit.Speed = SpeedNormal
	lowInit.Energy = BaseActionCost
	highInit.Energy = BaseActionCost
	_ = w.EnqueueIntent("aaaa", MoveIntent{DX: 0, DY: 1})
	_ = w.EnqueueIntent("bbbb", MoveIntent{DX: 0, DY: 1})

	events := w.Tick()

	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}
	if id := events[0].(EntityMovedEvent).EntityID; id != "bbbb" {
		t.Errorf("first event EntityID = %q, want bbbb (Initiative desc)", id)
	}
}

// TestTickOneActionPerEntityPerTick locks in the one-action-per-tick cap:
// even a fast entity (Speed > BaseActionCost) that has an Intent ready at
// tick start resolves exactly one action and the Intent slot is then
// cleared, so a stale Intent cannot fire twice inside the same Tick.
func TestTickOneActionPerEntityPerTick(t *testing.T) {
	w := newTickTestWorld(t)
	p := joinPlayer(t, w)
	p.Speed = SpeedVeryFast // 24 — clamp would permit two actions worth of Speed
	p.Energy = BaseActionCost
	_ = w.EnqueueIntent("p1", MoveIntent{DX: 1, DY: 0})

	events := w.Tick()

	if len(events) != 1 {
		t.Fatalf("events = %d, want 1 (one action per tick cap)", len(events))
	}
	_ = firstMoved(t, events)
	// min(12 + 24, 12) = 12, resolve, 12 - 12 = 0.
	if p.Energy != 0 {
		t.Errorf("Energy = %d, want 0 (clamp then Cost deducted)", p.Energy)
	}
	if p.Intent != nil {
		t.Errorf("Intent = %v, want nil (consumed, not auto-rearmed)", p.Intent)
	}
}

func TestTickMultipleTicksMultipleMoves(t *testing.T) {
	w := newTickTestWorld(t)
	slow := joinPlayerAt(t, w, "aaaa", "Slow")
	fast := joinPlayerAt(t, w, "bbbb", "Fast")
	slow.Speed = SpeedNormal
	fast.Speed = SpeedVeryFast
	slow.Energy = BaseActionCost
	fast.Energy = BaseActionCost

	const ticks = 5
	slowMoves, fastMoves := 0, 0
	for range ticks {
		_ = w.EnqueueIntent("aaaa", MoveIntent{DX: 0, DY: 1})
		_ = w.EnqueueIntent("bbbb", MoveIntent{DX: 0, DY: 1})
		for _, ev := range w.Tick() {
			em, ok := ev.(EntityMovedEvent)
			if !ok {
				continue
			}
			switch em.EntityID {
			case "aaaa":
				slowMoves++
			case "bbbb":
				fastMoves++
			}
		}
	}

	if slowMoves != ticks {
		t.Errorf("slow moves = %d, want %d (one per tick with re-enqueue)",
			slowMoves, ticks)
	}
	// The cap is one action per entity per tick regardless of surplus
	// Energy — fast also moves exactly `ticks` times even though Speed
	// would otherwise let it queue two actions.
	if fastMoves != ticks {
		t.Errorf("fast moves = %d, want %d (one-action-per-tick cap)",
			fastMoves, ticks)
	}
}

func TestTickResolveUnknownIntent(t *testing.T) {
	// Sanity coverage for resolveIntent's default branch: an unsupported
	// intent type must surface as an IntentFailedEvent with the refund
	// semantics, not a crash.
	w := newTickTestWorld(t)
	p := joinPlayer(t, w)
	p.Intent = stubIntent{}

	events := w.Tick()

	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	if _, ok := events[0].(IntentFailedEvent); !ok {
		t.Fatalf("event = %T, want IntentFailedEvent", events[0])
	}
	if p.Intent != nil {
		t.Errorf("Intent = %v, want nil (consumed even on unknown type)", p.Intent)
	}
}

// TestMcalcMoveDistribution verifies that mcalcMove produces the correct
// long-run average and bimodal distribution for a fractional speed. With
// Speed=9 and BaseActionCost=12 the expected gain per tick is exactly 9
// (0 × 3/12 + 12 × 9/12). Over 10 000 draws the sample mean must be
// within 5% of that expectation, and both possible outputs (0 and 12) must
// appear — proving the distribution is not degenerate.
func TestMcalcMoveDistribution(t *testing.T) {
	w := NewWorld(tickTestSource{}, WithSeed(42))

	const (
		draws    = 10_000
		speed    = 9
		wantMean = float64(speed) // E[mcalcMove(9)] = 9
		tol      = 0.05           // 5% relative tolerance
	)

	var total int
	saw0, saw12 := false, false
	for range draws {
		g := w.mcalcMove(speed)
		total += g
		if g == 0 {
			saw0 = true
		}
		if g == BaseActionCost {
			saw12 = true
		}
	}

	got := float64(total) / float64(draws)
	if got < wantMean*(1-tol) || got > wantMean*(1+tol) {
		t.Errorf("mcalcMove(%d) mean = %.4f, want %.4f ± %.0f%%", speed, got, wantMean, tol*100)
	}
	if !saw0 {
		t.Errorf("mcalcMove(%d) never returned 0; distribution appears non-bimodal", speed)
	}
	if !saw12 {
		t.Errorf("mcalcMove(%d) never returned %d; distribution appears non-bimodal", speed, BaseActionCost)
	}
}

// stubIntent is an Intent concrete type unknown to resolveIntent's
// switch, used only by TestTickResolveUnknownIntent.
type stubIntent struct{}

func (stubIntent) isIntent() {}
func (stubIntent) Cost() int { return BaseActionCost }

// newTestMonster creates a Monster with sensible tick-ready defaults for
// use in M4 tests. Speed and Energy must be set by the caller when the
// test needs specific values.
func newTestMonster(t *testing.T, id, name string) *Monster {
	t.Helper()
	m, err := NewMonster(id, name, DefaultCoreStats())
	if err != nil {
		t.Fatalf("NewMonster %s: %v", id, err)
	}
	m.Speed = SpeedNormal
	m.Energy = 0
	return m
}

// TestTickAccumulatesMonsterEnergy verifies that a monster with no Intent
// saturates at the idle clamp (BaseActionCost) after enough ticks to reach
// it, and then holds steady regardless of how many additional ticks pass.
// Without the clamp, an idle monster would drift Energy upward forever.
func TestTickAccumulatesMonsterEnergy(t *testing.T) {
	w := newTickTestWorld(t)
	m := newTestMonster(t, "m1", "Zombie")
	w.AddMonster(m)

	const ticks = 3
	for range ticks {
		evs := w.Tick()
		if len(evs) != 0 {
			t.Fatalf("tick produced %d events, want 0 (monster has no intent)", len(evs))
		}
	}

	if m.Energy != BaseActionCost {
		t.Errorf("Energy = %d, want %d (idle clamp to BaseActionCost)",
			m.Energy, BaseActionCost)
	}
	if m.Intent != nil {
		t.Errorf("Intent = %v, want nil", m.Intent)
	}
}

// TestTickResolvesMonsterMoveIntent sets a MoveIntent directly on a
// monster (simulating a future AI setting it) and verifies that Tick
// emits an EntityMovedEvent, deducts Energy, and clears the intent.
func TestTickResolvesMonsterMoveIntent(t *testing.T) {
	w := newTickTestWorld(t)
	m := newTestMonster(t, "m1", "Wolf")
	m.Energy = BaseActionCost // pre-charged so it fires on the first tick
	w.AddMonster(m)

	m.Intent = MoveIntent{DX: 1, DY: 0}

	evs := w.Tick()

	if len(evs) != 1 {
		t.Fatalf("events = %d, want 1 EntityMovedEvent", len(evs))
	}
	em, ok := evs[0].(EntityMovedEvent)
	if !ok {
		t.Fatalf("event = %T, want EntityMovedEvent", evs[0])
	}
	if em.EntityID != "m1" {
		t.Errorf("EntityID = %q, want m1", em.EntityID)
	}
	if m.Intent != nil {
		t.Errorf("Intent = %v, want nil (consumed)", m.Intent)
	}
	// Energy: accumulation clamped at BaseActionCost, then full cost
	// deducted on successful resolution → exactly zero.
	if m.Energy != 0 {
		t.Errorf("Energy = %d, want 0 (clamp then Cost deducted)", m.Energy)
	}
}

// TestTickOrdersPlayersAndMonstersTogether checks that a monster with
// higher Speed is ordered before a slower player within the same tick.
func TestTickOrdersPlayersAndMonstersTogether(t *testing.T) {
	w := newTickTestWorld(t)

	player := joinPlayerAt(t, w, "p-slow", "Slow")
	player.Speed = SpeedNormal
	player.Energy = BaseActionCost
	_ = w.EnqueueIntent("p-slow", MoveIntent{DX: 0, DY: 1})

	monster := newTestMonster(t, "m-fast", "FastWolf")
	monster.Speed = SpeedVeryFast
	monster.Energy = BaseActionCost
	monster.Intent = MoveIntent{DX: 0, DY: 1}
	// Place the monster away from the player's spawn so both moves resolve —
	// collision semantics now block a monster and a player from sharing a tile.
	monster.Position = Position{X: 2, Y: 0}
	w.AddMonster(monster)

	evs := w.Tick()

	if len(evs) != 2 {
		t.Fatalf("events = %d, want 2", len(evs))
	}
	firstID := evs[0].(EntityMovedEvent).EntityID
	if firstID != "m-fast" {
		t.Errorf("first event EntityID = %q, want m-fast (higher Speed goes first)", firstID)
	}
	secondID := evs[1].(EntityMovedEvent).EntityID
	if secondID != "p-slow" {
		t.Errorf("second event EntityID = %q, want p-slow", secondID)
	}
}

// TestMonsterMoveBlocked verifies that a monster attempting to walk
// onto an impassable tile fails the move: the tick emits an
// IntentFailedEvent, the monster's Position does not change, and
// Energy is refunded (the refund semantics live in Tick, not in
// applyMonsterMoveIntent, which only emits events).
func TestMonsterMoveBlocked(t *testing.T) {
	wall := Position{X: 1, Y: 0}
	w := NewWorld(wallAtSource{wall: wall})

	monster := newTestMonster(t, "m1", "Ogre")
	monster.Energy = BaseActionCost // pre-charged so it fires on first tick
	monster.Intent = MoveIntent{DX: 1, DY: 0}
	w.AddMonster(monster)

	evs := w.Tick()
	if len(evs) != 1 {
		t.Fatalf("events = %d, want 1 IntentFailedEvent", len(evs))
	}
	fail, ok := evs[0].(IntentFailedEvent)
	if !ok {
		t.Fatalf("event = %T, want IntentFailedEvent", evs[0])
	}
	if fail.EntityID != "m1" {
		t.Errorf("EntityID = %q, want m1", fail.EntityID)
	}
	if fail.Reason != ReasonIntentMoveBlocked {
		t.Errorf("Reason = %q, want %q", fail.Reason, ReasonIntentMoveBlocked)
	}
	if monster.Position != (Position{X: 0, Y: 0}) {
		t.Errorf("Position = %+v, want origin (blocked move must not mutate)",
			monster.Position)
	}
	// Refund on failure keeps Energy at the clamp ceiling: accumulation
	// capped at BaseActionCost, no Cost deduction.
	if monster.Energy != BaseActionCost {
		t.Errorf("Energy = %d, want %d (clamp, no Cost deducted on fail)",
			monster.Energy, BaseActionCost)
	}
}

// TestMonsterMoveBlockedByPlayer verifies that a monster cannot step
// onto a tile a player occupies. Symmetric to the player-vs-player
// occupancy check, now extended across species.
func TestMonsterMoveBlockedByPlayer(t *testing.T) {
	w := newTickTestWorld(t)

	// Player spawns at origin via findSpawn, so target tile is {1,0}.
	player, err := NewPlayer("p1", "Alice", DefaultCoreStats(), Position{X: 1, Y: 0})
	if err != nil {
		t.Fatalf("new player: %v", err)
	}
	w.players[player.ID] = player
	w.positions[player.ID] = player.Position
	w.occupants[player.Position] = player

	monster := newTestMonster(t, "m1", "Goblin")
	monster.Energy = BaseActionCost
	monster.Intent = MoveIntent{DX: 1, DY: 0}
	w.AddMonster(monster)

	evs := w.Tick()
	if len(evs) != 1 {
		t.Fatalf("events = %d, want 1 IntentFailedEvent", len(evs))
	}
	if _, ok := evs[0].(IntentFailedEvent); !ok {
		t.Fatalf("event = %T, want IntentFailedEvent", evs[0])
	}
	if monster.Position != (Position{}) {
		t.Errorf("monster Position = %+v, want origin", monster.Position)
	}
}

// TestPlayerMoveBlockedByMonster verifies the symmetric check: a
// player cannot step onto a tile a monster occupies.
func TestPlayerMoveBlockedByMonster(t *testing.T) {
	w := newTickTestWorld(t)

	player := joinPlayerAt(t, w, "p1", "Alice")
	player.Energy = BaseActionCost

	monster := newTestMonster(t, "m1", "Wolf")
	monster.Position = Position{X: 1, Y: 0}
	w.AddMonster(monster)

	if err := w.EnqueueIntent("p1", MoveIntent{DX: 1, DY: 0}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	evs := w.Tick()
	if len(evs) != 1 {
		t.Fatalf("events = %d, want 1 IntentFailedEvent", len(evs))
	}
	fail, ok := evs[0].(IntentFailedEvent)
	if !ok {
		t.Fatalf("event = %T, want IntentFailedEvent", evs[0])
	}
	if fail.EntityID != "p1" {
		t.Errorf("EntityID = %q, want p1", fail.EntityID)
	}
	if fail.Reason != ReasonIntentMoveBlocked {
		t.Errorf("Reason = %q, want %q", fail.Reason, ReasonIntentMoveBlocked)
	}
}

// TestAddMonsterIdempotent verifies that adding a monster with the same
// ID twice replaces the first entry rather than panicking or duplicating.
func TestAddMonsterIdempotent(t *testing.T) {
	w := newTickTestWorld(t)

	m1 := &Monster{ID: "x", Name: "First", Stats: DefaultCoreStats(), Speed: SpeedNormal}
	m2 := &Monster{ID: "x", Name: "Second", Stats: DefaultCoreStats(), Speed: SpeedFast}
	w.AddMonster(m1)
	w.AddMonster(m2)

	monsters := w.Monsters()
	if len(monsters) != 1 {
		t.Fatalf("len(monsters) = %d, want 1", len(monsters))
	}
	if got := monsters["x"].Name; got != "Second" {
		t.Errorf("monster name = %q, want Second (second add overwrites first)", got)
	}
}

// TestRemoveMonsterNoOp verifies that RemoveMonster on an unknown id does
// not panic.
func TestRemoveMonsterNoOp(t *testing.T) {
	w := newTickTestWorld(t)
	// Must not panic.
	w.RemoveMonster("nonexistent")

	m := newTestMonster(t, "m1", "Ghost")
	w.AddMonster(m)
	w.RemoveMonster("m1")
	if _, ok := w.Monsters()["m1"]; ok {
		t.Errorf("monster still present after RemoveMonster")
	}
}
