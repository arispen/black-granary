package main

import (
	"regexp"
	"testing"
	"time"
)

func TestCrisisLifecycleBranches(t *testing.T) {
	s := newTestStore()
	now := time.Now().UTC()
	def, ok := crisisDefinitionByType("plague")
	if !ok {
		t.Fatalf("expected plague definition")
	}

	startCrisisLocked(s, def, now)
	if s.ActiveCrisis == nil || s.ActiveCrisis.Type != def.Type {
		t.Fatalf("startCrisisLocked should set active crisis")
	}
	if len(s.Events) == 0 {
		t.Fatalf("startCrisisLocked should emit event")
	}

	resolver := &Player{ID: "p1", Name: "Resolver"}
	resolveCrisisLocked(s, def, now, true, resolver)
	if s.ActiveCrisis != nil {
		t.Fatalf("mitigated crisis should clear active crisis")
	}
	if len(s.Events) == 0 || s.Events[len(s.Events)-1].Type != "Crisis" {
		t.Fatalf("mitigated resolution should emit crisis event")
	}

	s.World.UnrestValue = 10
	s.World.UnrestTier = unrestTierFromValue(s.World.UnrestValue)
	s.World.GrainSupply = 50
	s.World.GrainTier = grainTierFromSupply(s.World.GrainSupply)
	s.ActiveCrisis = &Crisis{Type: def.Type, Name: def.Name, TicksLeft: 0}
	resolveCrisisLocked(s, def, now, false, nil)
	if s.ActiveCrisis != nil {
		t.Fatalf("failed crisis should clear active crisis")
	}
	if s.World.UnrestValue <= 10 {
		t.Fatalf("failed crisis should increase unrest")
	}
	if s.World.GrainSupply >= 50 {
		t.Fatalf("failed crisis should reduce grain")
	}

	prior := s.ActiveCrisis
	s.ActiveCrisis = &Crisis{Type: "fire", Name: "Warehouse Inferno", TicksLeft: 2}
	maybeStartCrisisLocked(s, now)
	if s.ActiveCrisis != nil && s.ActiveCrisis.Type != "fire" {
		t.Fatalf("maybeStartCrisisLocked should not replace existing crisis")
	}
	_ = prior

	s.ActiveCrisis = nil
	s.World.UnrestTier = "Rioting"
	s.World.GrainTier = "Critical"
	started := false
	for i := 0; i < 1000; i++ {
		maybeStartCrisisLocked(s, now)
		if s.ActiveCrisis != nil {
			started = true
			break
		}
	}
	if !started {
		t.Fatalf("expected maybeStartCrisisLocked to eventually start a crisis")
	}
}

func TestActiveContractHelperBranches(t *testing.T) {
	s := newTestStore()
	s.Contracts["s1"] = &Contract{ID: "s1", Type: "Supply", IssuerPlayerID: "issuer", Status: "Failed"}
	if hasActiveSupplyFromIssuerLocked(s, "issuer") {
		t.Fatalf("failed supply should not be active")
	}
	s.Contracts["s1"].Status = "Fulfilled"
	if !hasActiveSupplyFromIssuerLocked(s, "issuer") {
		t.Fatalf("fulfilled supply should be active for issuer checks")
	}

	s.Contracts["e1"] = &Contract{ID: "e1", Type: "Emergency", Status: "Completed"}
	if hasActiveContractLocked(s, "Emergency") {
		t.Fatalf("completed contract should not be active")
	}
	s.Contracts["e1"].Status = "Issued"
	if !hasActiveContractLocked(s, "Emergency") {
		t.Fatalf("issued contract should be active")
	}

	s.Contracts["b1"] = &Contract{ID: "b1", Type: "Bounty", TargetPlayerID: "target", Status: "Cancelled"}
	if hasActiveBountyForTargetLocked(s, "target") {
		t.Fatalf("cancelled bounty should not be active")
	}
	s.Contracts["b1"].Status = "Accepted"
	if !hasActiveBountyForTargetLocked(s, "target") {
		t.Fatalf("accepted bounty should be active")
	}
}

func TestMiscLowCoverageHelpers(t *testing.T) {
	if got := seatDefaultHolderName("harbor_master"); got != "Captain Vey (NPC)" {
		t.Fatalf("harbor master default = %q", got)
	}
	if got := seatDefaultHolderName("master_of_coin"); got != "Clerk Marn (NPC)" {
		t.Fatalf("master of coin default = %q", got)
	}
	if got := seatDefaultHolderName("watch_commander"); got != "Marshal Dain (NPC)" {
		t.Fatalf("watch commander default = %q", got)
	}
	if got := seatDefaultHolderName("high_curate"); got != "Sister Hal (NPC)" {
		t.Fatalf("high curate default = %q", got)
	}
	if got := seatDefaultHolderName("unknown"); got != "Appointee (NPC)" {
		t.Fatalf("default seat holder = %q", got)
	}

	if got := fulfillChanceForTier("Stable"); got != 70 {
		t.Fatalf("fulfill chance stable = %d", got)
	}
	if got := fulfillChanceForTier("Tight"); got != 55 {
		t.Fatalf("fulfill chance tight = %d", got)
	}
	if got := fulfillChanceForTier("Scarce"); got != 40 {
		t.Fatalf("fulfill chance scarce = %d", got)
	}
	if got := fulfillChanceForTier("x"); got != 25 {
		t.Fatalf("fulfill chance default = %d", got)
	}

	s := newTestStore()
	s.TickCount = 10
	if got := fieldworkCooldownRemaining(s, "p1"); got != 0 {
		t.Fatalf("cooldown without record = %d", got)
	}
	s.LastFieldworkAt["p1"] = 9
	if got := fieldworkCooldownRemaining(s, "p1"); got != 1 {
		t.Fatalf("cooldown one tick after = %d", got)
	}
	s.LastFieldworkAt["p1"] = 6
	if got := fieldworkCooldownRemaining(s, "p1"); got != 0 {
		t.Fatalf("cooldown should be cleared = %d", got)
	}

	suffix := randomSuffix()
	if len(suffix) != 3 {
		t.Fatalf("randomSuffix length = %d, value=%q", len(suffix), suffix)
	}
	if !regexp.MustCompile(`^[A-Z0-9_-]{3}$`).MatchString(suffix) {
		t.Fatalf("randomSuffix format unexpected: %q", suffix)
	}
}
