package main

import (
	"fmt"
	mathrand "math/rand"
	"net/http/httptest"
	"testing"
	"time"
)

func newTestStore() *Store {
	s := newStore()
	s.rng = mathrand.New(mathrand.NewSource(1))
	return s
}

func TestTierThresholds(t *testing.T) {
	tests := []struct {
		supply     int
		grainTier  string
		unrest     int
		unrestTier string
	}{
		{201, "Stable", 10, "Calm"},
		{200, "Tight", 11, "Uneasy"},
		{101, "Tight", 30, "Uneasy"},
		{100, "Scarce", 31, "Unstable"},
		{41, "Scarce", 60, "Unstable"},
		{40, "Critical", 61, "Rioting"},
	}

	for _, tc := range tests {
		if got := grainTierFromSupply(tc.supply); got != tc.grainTier {
			t.Fatalf("grainTierFromSupply(%d) = %q, want %q", tc.supply, got, tc.grainTier)
		}
		if got := unrestTierFromValue(tc.unrest); got != tc.unrestTier {
			t.Fatalf("unrestTierFromValue(%d) = %q, want %q", tc.unrest, got, tc.unrestTier)
		}
	}
}

func TestReputationTitleAndPayoutBounds(t *testing.T) {
	if got := reputationTitle(50); got != "Renowned" {
		t.Fatalf("title for 50 = %q", got)
	}
	if got := reputationTitle(-50); got != "Notorious" {
		t.Fatalf("title for -50 = %q", got)
	}
	if got := payoutMultiplier(-100); got != 0.75 {
		t.Fatalf("payoutMultiplier(-100) = %v, want 0.75", got)
	}
	if got := payoutMultiplier(100); got != 1.5 {
		t.Fatalf("payoutMultiplier(100) = %v, want 1.5", got)
	}
}

func TestStandingLabelMappingsAtBoundaries(t *testing.T) {
	repCases := []struct {
		rep  int
		want string
	}{
		{-20, "Pariah"},
		{-19, "Disliked"},
		{-6, "Disliked"},
		{-5, "Unknown"},
		{5, "Unknown"},
		{6, "Trusted"},
		{19, "Trusted"},
		{20, "Esteemed"},
	}
	for _, tc := range repCases {
		if got := standingReputationLabel(tc.rep); got != tc.want {
			t.Fatalf("standingReputationLabel(%d) = %q, want %q", tc.rep, got, tc.want)
		}
	}

	heatCases := []struct {
		heat int
		want string
	}{
		{0, "Clean"},
		{1, "Watched"},
		{4, "Watched"},
		{5, "Wanted"},
	}
	for _, tc := range heatCases {
		if got := standingHeatLabel(tc.heat); got != tc.want {
			t.Fatalf("standingHeatLabel(%d) = %q, want %q", tc.heat, got, tc.want)
		}
	}
}

func TestAcceptRulesAndSingleContractLimit(t *testing.T) {
	s := newTestStore()
	now := time.Now().UTC()

	p1 := &Player{ID: "p1", Name: "Ash Crow (Guest)", Gold: 20, Rep: 0, LastSeen: now}
	p2 := &Player{ID: "p2", Name: "Bran Vale (Guest)", Gold: 20, Rep: 0, LastSeen: now}
	s.Players[p1.ID] = p1
	s.Players[p2.ID] = p2

	s.Contracts["c1"] = &Contract{ID: "c1", Type: "Emergency", DeadlineTicks: 3, Status: "Issued"}
	handleActionLocked(s, p1, now, "accept", "c1")

	if s.Contracts["c1"].Status != "Accepted" || s.Contracts["c1"].OwnerPlayerID != p1.ID {
		t.Fatalf("expected c1 accepted by p1, got status=%s owner=%s", s.Contracts["c1"].Status, s.Contracts["c1"].OwnerPlayerID)
	}
	if s.Contracts["c1"].Stance != contractStanceCareful {
		t.Fatalf("accept should default to Careful stance, got %q", s.Contracts["c1"].Stance)
	}

	handleActionLocked(s, p2, now.Add(3*time.Second), "accept", "c1")
	if got := s.ToastByPlayer[p2.ID]; got == "" {
		t.Fatalf("expected toast for taken contract")
	}

	s.Contracts["c2"] = &Contract{ID: "c2", Type: "Emergency", DeadlineTicks: 3, Status: "Issued"}
	handleActionLocked(s, p1, now.Add(4*time.Second), "accept", "c2")
	if s.Contracts["c2"].Status != "Issued" {
		t.Fatalf("player should not hold two accepted contracts")
	}
}

func TestInvestigateCooldownByTicks(t *testing.T) {
	s := newTestStore()
	now := time.Now().UTC()
	p := &Player{ID: "p1", Name: "Ash Crow (Guest)", Gold: 20, Rep: 0, LastSeen: now}
	s.Players[p.ID] = p
	s.World.UnrestValue = 20

	handleActionLocked(s, p, now, "investigate", "")
	if s.World.UnrestValue != 15 || p.Rep != 1 {
		t.Fatalf("first investigate should be effective, unrest=%d rep=%d", s.World.UnrestValue, p.Rep)
	}

	handleActionLocked(s, p, now.Add(3*time.Second), "investigate", "")
	if s.World.UnrestValue != 15 || p.Rep != 1 {
		t.Fatalf("second investigate in same tick window should be flavor only")
	}

	s.TickCount += 3
	handleActionLocked(s, p, now.Add(6*time.Second), "investigate", "")
	if s.World.UnrestValue != 10 || p.Rep != 2 {
		t.Fatalf("investigate after 3 ticks should be effective again, unrest=%d rep=%d", s.World.UnrestValue, p.Rep)
	}
}

func TestDeliverCostNeverNegative(t *testing.T) {
	s := newTestStore()
	now := time.Now().UTC()
	p := &Player{ID: "p1", Name: "Ash Crow (Guest)", Gold: 1, Rep: 0, LastSeen: now}
	s.Players[p.ID] = p
	s.Contracts["c1"] = &Contract{ID: "c1", Type: "Emergency", DeadlineTicks: 3, Status: "Accepted", OwnerPlayerID: p.ID, OwnerName: p.Name}

	handleActionLocked(s, p, now, "deliver", "c1")
	if p.Gold != 0 {
		t.Fatalf("deliver should clamp gold to 0, got %d", p.Gold)
	}
}

func TestDeliveryCompletionIncrementsImpactAndHeat(t *testing.T) {
	s := newTestStore()
	now := time.Now().UTC()
	p := &Player{ID: "p1", Name: "Ash Crow (Guest)", Gold: 20, Rep: 0, LastSeen: now}
	s.Players[p.ID] = p
	s.Contracts["c1"] = &Contract{ID: "c1", Type: "Smuggling", DeadlineTicks: 3, Status: "Fulfilled", OwnerPlayerID: p.ID, OwnerName: p.Name, Stance: contractStanceCareful}

	handleActionLocked(s, p, now, "deliver", "c1")

	if got := s.Contracts["c1"].Status; got != "Completed" {
		t.Fatalf("fulfilled delivery should close contract, got status=%s", got)
	}
	if p.CompletedContracts != 1 || p.CompletedContractsToday != 1 {
		t.Fatalf("expected impact counters incremented, total=%d today=%d", p.CompletedContracts, p.CompletedContractsToday)
	}
	if p.Heat != 1 {
		t.Fatalf("smuggling completion should increase heat with clamping/modifiers, got %d", p.Heat)
	}

	later := now.Add(25 * time.Hour)
	_ = buildPageDataLocked(s, p.ID, false)
	ensureTodayCounterLocked(p, later)
	if p.CompletedContracts != 1 {
		t.Fatalf("lifetime counter should persist, got %d", p.CompletedContracts)
	}
	if p.CompletedContractsToday != 0 {
		t.Fatalf("today counter should reset on UTC date change, got %d", p.CompletedContractsToday)
	}
}

func TestContractActionVisibilityByState(t *testing.T) {
	s := newTestStore()
	now := time.Now().UTC()
	p := &Player{ID: "p1", Name: "Ash Crow (Guest)", Gold: 20, Rep: 0, LastSeen: now}
	other := &Player{ID: "p2", Name: "Bran Vale (Guest)", Gold: 20, Rep: 0, LastSeen: now}
	s.Players[p.ID] = p
	s.Players[other.ID] = other

	s.Contracts["issued"] = &Contract{ID: "issued", Type: "Emergency", DeadlineTicks: 3, Status: "Issued"}
	s.Contracts["accepted"] = &Contract{ID: "accepted", Type: "Emergency", DeadlineTicks: 3, Status: "Accepted", OwnerPlayerID: p.ID, OwnerName: p.Name, Stance: contractStanceCareful}
	s.Contracts["fulfilled"] = &Contract{ID: "fulfilled", Type: "Emergency", DeadlineTicks: 3, Status: "Fulfilled", OwnerPlayerID: p.ID, OwnerName: p.Name, Stance: contractStanceCareful}
	s.Contracts["completed"] = &Contract{ID: "completed", Type: "Emergency", DeadlineTicks: 3, Status: "Completed", OwnerPlayerID: p.ID, OwnerName: p.Name}

	data := buildPageDataLocked(s, p.ID, false)
	views := map[string]ContractView{}
	for _, cv := range data.Contracts {
		views[cv.ID] = cv
	}

	if !views["issued"].CanAccept || !views["issued"].CanIgnore || views["issued"].CanDeliver || views["issued"].CanAbandon {
		t.Fatalf("issued contract should show only accept/ignore: %#v", views["issued"])
	}
	if !views["accepted"].CanDeliver || !views["accepted"].CanAbandon || views["accepted"].CanAccept || views["accepted"].CanIgnore {
		t.Fatalf("accepted contract should show deliver/abandon: %#v", views["accepted"])
	}
	if views["accepted"].DeliverLabel != "Deliver (+20g)" {
		t.Fatalf("accepted contract deliver label mismatch: %q", views["accepted"].DeliverLabel)
	}
	if !views["fulfilled"].CanDeliver || views["fulfilled"].CanAbandon || views["fulfilled"].CanAccept || views["fulfilled"].CanIgnore {
		t.Fatalf("fulfilled contract should show deliver only: %#v", views["fulfilled"])
	}
	if views["fulfilled"].DeliverLabel != "Deliver (+22g)" {
		t.Fatalf("fulfilled contract deliver label mismatch: %q", views["fulfilled"].DeliverLabel)
	}
	if views["completed"].CanDeliver || views["completed"].CanAbandon || views["completed"].CanAccept || views["completed"].CanIgnore {
		t.Fatalf("completed contract should show no actions: %#v", views["completed"])
	}
}

func TestContractsAreCappedInDashboardData(t *testing.T) {
	s := newTestStore()
	now := time.Now().UTC()
	p := &Player{ID: "p1", Name: "Ash Crow (Guest)", Gold: 20, Rep: 0, LastSeen: now}
	s.Players[p.ID] = p

	for i := 1; i <= maxVisibleContracts+8; i++ {
		id := fmt.Sprintf("c-%d", i)
		s.Contracts[id] = &Contract{ID: id, Type: "Emergency", DeadlineTicks: 3, Status: "Completed"}
	}
	for i := 1; i <= 3; i++ {
		id := fmt.Sprintf("active-%d", i)
		s.Contracts[id] = &Contract{ID: id, Type: "Emergency", DeadlineTicks: 2, Status: "Issued"}
	}

	data := buildPageDataLocked(s, p.ID, false)
	if data.TotalContractN != maxVisibleContracts+11 {
		t.Fatalf("unexpected total contracts count: %d", data.TotalContractN)
	}
	if data.VisibleContractN != maxVisibleContracts {
		t.Fatalf("visible contracts should be capped at %d, got %d", maxVisibleContracts, data.VisibleContractN)
	}
	if len(data.Contracts) != maxVisibleContracts {
		t.Fatalf("contract views should be capped at %d, got %d", maxVisibleContracts, len(data.Contracts))
	}
}

func TestResolveWhisperTargetSupportsGuestAndPlainName(t *testing.T) {
	s := newTestStore()
	now := time.Now().UTC()
	ash := &Player{ID: "p1", Name: "Ash Crow (Guest)", Gold: 20, Rep: 0, LastSeen: now}
	s.Players[ash.ID] = ash
	s.Players["p2"] = &Player{ID: "p2", Name: "Bran Vale (Guest)", Gold: 20, Rep: 0, LastSeen: now}

	target, body := resolveWhisperTargetLocked(s, "Ash Crow hello there")
	if target == nil || target.ID != ash.ID || body != "hello there" {
		t.Fatalf("plain whisper parse failed: target=%v body=%q", target, body)
	}

	target, body = resolveWhisperTargetLocked(s, "Ash Crow (Guest) check route")
	if target == nil || target.ID != ash.ID || body != "check route" {
		t.Fatalf("guest whisper parse failed: target=%v body=%q", target, body)
	}
}

func TestTickTimeProgressionAndInactivityAutoAbandon(t *testing.T) {
	s := newTestStore()
	now := time.Now().UTC()
	owner := &Player{ID: "p1", Name: "Ash Crow (Guest)", Gold: 20, Rep: 0, LastSeen: now.Add(-inactiveWindow - 1*time.Second)}
	s.Players[owner.ID] = owner
	s.Contracts["c1"] = &Contract{ID: "c1", Type: "Emergency", DeadlineTicks: 3, Status: "Accepted", OwnerPlayerID: owner.ID, OwnerName: owner.Name, Stance: contractStanceFast}

	if s.World.DayNumber != 1 || s.World.Subphase != "Morning" {
		t.Fatalf("unexpected initial world time")
	}

	runWorldTickLocked(s, now)
	if s.World.Subphase != "Evening" || s.World.DayNumber != 1 {
		t.Fatalf("expected Morning->Evening same day, got day=%d subphase=%s", s.World.DayNumber, s.World.Subphase)
	}
	if s.Contracts["c1"].Status != "Issued" || s.Contracts["c1"].OwnerPlayerID != "" {
		t.Fatalf("inactive owner should auto-abandon contract, got status=%s owner=%s", s.Contracts["c1"].Status, s.Contracts["c1"].OwnerPlayerID)
	}
	if s.Contracts["c1"].Stance != "" {
		t.Fatalf("inactive auto-abandon should clear stance, got %q", s.Contracts["c1"].Stance)
	}

	runWorldTickLocked(s, now.Add(time.Minute))
	if s.World.Subphase != "Morning" || s.World.DayNumber != 2 {
		t.Fatalf("expected Evening->Morning and day increment, got day=%d subphase=%s", s.World.DayNumber, s.World.Subphase)
	}
}

func TestIsAdminTokenAndLoopback(t *testing.T) {
	req := httptest.NewRequest("GET", "http://example.test/admin?token=DEV", nil)
	req.RemoteAddr = "203.0.113.10:12345"
	if !isAdmin(req) {
		t.Fatalf("token auth should allow admin")
	}

	req2 := httptest.NewRequest("GET", "http://example.test/admin", nil)
	req2.RemoteAddr = "127.0.0.1:45678"
	if !isAdmin(req2) {
		t.Fatalf("loopback should allow admin")
	}
}

func TestDeliverOutcomeRewardRoundingByStance(t *testing.T) {
	p := &Player{Rep: 0}
	c := &Contract{Type: "Emergency"}

	c.Stance = contractStanceCareful
	if got := computeDeliverOutcomeLocked(p, c).RewardGold; got != 22 {
		t.Fatalf("careful reward = %d, want 22", got)
	}
	c.Stance = contractStanceFast
	if got := computeDeliverOutcomeLocked(p, c).RewardGold; got != 27 {
		t.Fatalf("fast reward = %d, want 27", got)
	}
	c.Stance = contractStanceQuiet
	if got := computeDeliverOutcomeLocked(p, c).RewardGold; got != 20 {
		t.Fatalf("quiet reward = %d, want 20", got)
	}
}

func TestDeliverHeatClampAtZero(t *testing.T) {
	s := newTestStore()
	now := time.Now().UTC()
	p := &Player{ID: "p1", Name: "Ash Crow (Guest)", Gold: 20, Rep: 0, Heat: 0, LastSeen: now}
	s.Players[p.ID] = p
	s.Contracts["c1"] = &Contract{
		ID:            "c1",
		Type:          "Emergency",
		DeadlineTicks: 3,
		Status:        "Fulfilled",
		OwnerPlayerID: p.ID,
		OwnerName:     p.Name,
		Stance:        contractStanceQuiet,
	}

	handleActionLocked(s, p, now, "deliver", "c1")
	if p.Heat != 0 {
		t.Fatalf("heat should clamp at 0, got %d", p.Heat)
	}
}

func TestSmugglingExtraHeatStacksAfterStance(t *testing.T) {
	outcome := computeDeliverOutcomeLocked(
		&Player{Rep: 0},
		&Contract{Type: "Smuggling", Stance: contractStanceFast},
	)
	if outcome.HeatDelta != 4 {
		t.Fatalf("smuggling fast heat delta = %d, want 4", outcome.HeatDelta)
	}
}

func TestDeliverIdempotentDoesNotDoubleApply(t *testing.T) {
	s := newTestStore()
	now := time.Now().UTC()
	p := &Player{ID: "p1", Name: "Ash Crow (Guest)", Gold: 20, Rep: 0, Heat: 0, LastSeen: now}
	s.Players[p.ID] = p
	s.Contracts["c1"] = &Contract{
		ID:            "c1",
		Type:          "Emergency",
		DeadlineTicks: 3,
		Status:        "Fulfilled",
		OwnerPlayerID: p.ID,
		OwnerName:     p.Name,
		Stance:        contractStanceFast,
	}

	handleActionLocked(s, p, now, "deliver", "c1")
	goldAfterFirst := p.Gold
	repAfterFirst := p.Rep
	heatAfterFirst := p.Heat
	completedAfterFirst := p.CompletedContracts

	handleActionLocked(s, p, now.Add(time.Second), "deliver", "c1")
	if p.Gold != goldAfterFirst || p.Rep != repAfterFirst || p.Heat != heatAfterFirst {
		t.Fatalf("second deliver should not change resources: gold=%d rep=%d heat=%d", p.Gold, p.Rep, p.Heat)
	}
	if p.CompletedContracts != completedAfterFirst {
		t.Fatalf("second deliver should not increment completion, got %d", p.CompletedContracts)
	}
}
