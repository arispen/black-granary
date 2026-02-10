package main

import (
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
	s.Contracts["c1"] = &Contract{ID: "c1", Type: "Emergency", DeadlineTicks: 3, Status: "Accepted", OwnerPlayerID: owner.ID, OwnerName: owner.Name}

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
