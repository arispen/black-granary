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
		{1, "Noticed"},
		{3, "Noticed"},
		{4, "Watched"},
		{7, "Watched"},
		{8, "Wanted"},
		{11, "Wanted"},
		{12, "Hunted"},
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

func TestPermitAllowsEmergencyAcceptance(t *testing.T) {
	s := newTestStore()
	now := time.Now().UTC()

	harbor := &Player{ID: "p1", Name: "Harbor Master", Gold: 20, Rep: 30, LastSeen: now}
	contractor := &Player{ID: "p2", Name: "Bran Vale (Guest)", Gold: 20, Rep: 0, LastSeen: now}
	s.Players[harbor.ID] = harbor
	s.Players[contractor.ID] = contractor

	seat := s.Seats["harbor_master"]
	seat.HolderPlayerID = harbor.ID
	seat.HolderName = harbor.Name
	s.Policies.PermitRequiredHighRisk = true

	s.Contracts["c1"] = &Contract{ID: "c1", Type: "Emergency", DeadlineTicks: 3, Status: "Issued"}
	handleActionLocked(s, contractor, now, "accept", "c1")
	if s.Contracts["c1"].Status != "Issued" {
		t.Fatalf("expected permit to block acceptance without issuance")
	}

	handleActionInputLocked(s, harbor, now.Add(time.Second), ActionInput{Action: "issue_permit", TargetID: contractor.ID})
	if !hasActivePermitLocked(s, contractor.ID) {
		t.Fatalf("expected permit to be issued")
	}

	handleActionLocked(s, contractor, now.Add(2*time.Second), "accept", "c1")
	if s.Contracts["c1"].Status != "Accepted" || s.Contracts["c1"].OwnerPlayerID != contractor.ID {
		t.Fatalf("expected emergency contract accepted with permit")
	}
}

func TestPermitExpiresOnInstitutionTick(t *testing.T) {
	s := newTestStore()
	now := time.Now().UTC()
	p := &Player{ID: "p1", Name: "Ash Crow (Guest)", Gold: 20, Rep: 0, LastSeen: now}
	s.Players[p.ID] = p

	s.Permits[p.ID] = &Permit{
		PlayerID:   p.ID,
		PlayerName: p.Name,
		IssuerName: "Harbor Master",
		TicksLeft:  1,
		TotalTicks: 1,
	}
	processInstitutionTickLocked(s, now)
	if s.Permits[p.ID] != nil {
		t.Fatalf("expected permit to expire on tick")
	}
}

func TestInvestigateCooldownByTicks(t *testing.T) {
	s := newTestStore()
	now := time.Now().UTC()
	p := &Player{ID: "p1", Name: "Ash Crow (Guest)", Gold: 20, Rep: 0, LastSeen: now}
	s.Players[p.ID] = p
	s.World.UnrestValue = 20

	handleActionLocked(s, p, now, "investigate", "")
	if s.World.UnrestValue != 15 || p.Rep != 1 || p.Rumors != 1 {
		t.Fatalf("first investigate should be effective, unrest=%d rep=%d rumors=%d", s.World.UnrestValue, p.Rep, p.Rumors)
	}

	handleActionLocked(s, p, now.Add(3*time.Second), "investigate", "")
	if s.World.UnrestValue != 15 || p.Rep != 1 || p.Rumors != 1 {
		t.Fatalf("second investigate in same tick window should be flavor only")
	}

	s.TickCount += 3
	handleActionLocked(s, p, now.Add(6*time.Second), "investigate", "")
	if s.World.UnrestValue != 10 || p.Rep != 2 || p.Rumors != 2 {
		t.Fatalf("investigate after 3 ticks should be effective again, unrest=%d rep=%d rumors=%d", s.World.UnrestValue, p.Rep, p.Rumors)
	}
}

func TestPublishEvidenceDoesNotConsumeHighImpactWithoutEvidence(t *testing.T) {
	s := newTestStore()
	now := time.Now().UTC()
	p := &Player{ID: "p1", Name: "Ash Crow (Guest)", Gold: 20, Rep: 0, LastSeen: now}
	target := &Player{ID: "p2", Name: "Bran Vale (Guest)", Gold: 20, Rep: 0, LastSeen: now}
	s.Players[p.ID] = p
	s.Players[target.ID] = target

	s.DailyActionDate[p.ID] = now.Format("2006-01-02")
	s.DailyHighImpactN[p.ID] = 0

	handleActionInputLocked(s, p, now, ActionInput{Action: "publish_evidence", TargetID: target.ID})
	if s.DailyHighImpactN[p.ID] != 0 {
		t.Fatalf("expected high-impact budget to remain unused without evidence")
	}
}

func TestScryReportExpires(t *testing.T) {
	s := newTestStore()
	now := time.Now().UTC()
	owner := &Player{ID: "p1", Name: "Ash Crow (Guest)", Gold: 20, Rep: 0, LastSeen: now, LocationID: locationCapital}
	target := &Player{ID: "p2", Name: "Bran Vale (Guest)", Gold: 12, Grain: 3, Rep: 4, Heat: 2, LastSeen: now, LocationID: locationHarbor}
	s.Players[owner.ID] = owner
	s.Players[target.ID] = target

	s.TickCount = 10
	addScryReportLocked(s, owner, target)
	if len(s.ScryReports) != 1 {
		t.Fatalf("expected scry report to be added")
	}

	s.TickCount = 10 + scryReportDurationTicks
	processIntelTickLocked(s, now)
	if len(s.ScryReports) != 0 {
		t.Fatalf("expected scry reports to expire on tick")
	}
}

func TestInterceptCourierCapturesMessage(t *testing.T) {
	s := newTestStore()
	now := time.Now().UTC()
	interceptor := &Player{ID: "p1", Name: "Ash Crow (Guest)", Gold: 20, Rep: 100, LastSeen: now}
	target := &Player{ID: "p2", Name: "Bran Vale (Guest)", Gold: 12, Rep: 0, Heat: 20, LastSeen: now}
	other := &Player{ID: "p3", Name: "Corin Thorne (Guest)", Gold: 15, Rep: 0, LastSeen: now}
	s.Players[interceptor.ID] = interceptor
	s.Players[target.ID] = target
	s.Players[other.ID] = other

	s.Messages = append(s.Messages, DiplomaticMessage{
		ID:           1,
		FromPlayerID: target.ID,
		FromName:     target.Name,
		ToPlayerID:   other.ID,
		ToName:       other.Name,
		Subject:      "Quiet route",
		Body:         "Meet at dusk.",
		At:           now,
	})

	handleActionInputLocked(s, interceptor, now, ActionInput{Action: "intercept_courier", TargetID: target.ID})
	if len(s.Intercepts) != 1 {
		t.Fatalf("expected intercept to be created")
	}
}

func TestInterceptExpires(t *testing.T) {
	s := newTestStore()
	now := time.Now().UTC()
	owner := &Player{ID: "p1", Name: "Ash Crow (Guest)", Gold: 20, Rep: 0, LastSeen: now}
	s.Players[owner.ID] = owner
	msg := &DiplomaticMessage{
		ID:           1,
		FromPlayerID: "p2",
		FromName:     "Bran Vale (Guest)",
		ToPlayerID:   "p3",
		ToName:       "Corin Thorne (Guest)",
		Subject:      "Test",
		Body:         "Payload.",
		At:           now,
	}
	s.TickCount = 5
	addInterceptLocked(s, owner, msg)
	if len(s.Intercepts) != 1 {
		t.Fatalf("expected intercept to be added")
	}
	s.TickCount = 5 + interceptDurationTicks
	processIntelTickLocked(s, now)
	if len(s.Intercepts) != 0 {
		t.Fatalf("expected intercepts to expire on tick")
	}
}

func TestRelicAppraisalAndInvocation(t *testing.T) {
	s := newTestStore()
	now := time.Now().UTC()
	p := &Player{ID: "p1", Name: "Ash Crow (Guest)", Gold: 20, Rep: 0, Heat: 4, LocationID: locationCapital, LastSeen: now}
	s.Players[p.ID] = p

	relic := addRelicLocked(s, p, RelicDefinition{Name: "Warding Charm", Effect: "heat", Power: 2}, now)
	if relic == nil {
		t.Fatalf("expected relic to be created")
	}

	handleActionInputLocked(s, p, now, ActionInput{Action: "appraise_relic", RelicID: fmt.Sprintf("%d", relic.ID)})
	if relic.Status != relicStatusAppraised {
		t.Fatalf("expected relic to be appraised")
	}
	if p.Gold != 20-relicAppraiseCost {
		t.Fatalf("expected appraisal to cost %dg, got %dg", relicAppraiseCost, 20-p.Gold)
	}

	handleActionInputLocked(s, p, now, ActionInput{Action: "invoke_relic", RelicID: fmt.Sprintf("%d", relic.ID)})
	if p.Heat != 2 {
		t.Fatalf("expected relic invoke to reduce heat to 2, got %d", p.Heat)
	}
	if _, ok := s.Relics[relic.ID]; ok {
		t.Fatalf("expected relic to be removed after invocation")
	}
}

func TestCrisisResponseResolvesWithRewards(t *testing.T) {
	s := newTestStore()
	now := time.Now().UTC()
	p := &Player{ID: "p1", Name: "Ash Crow (Guest)", Gold: 10, Grain: 4, Rep: 0, LastSeen: now}
	s.Players[p.ID] = p

	def, ok := crisisDefinitionByType("plague")
	if !ok {
		t.Fatalf("expected plague crisis definition")
	}
	s.ActiveCrisis = &Crisis{
		Type:        def.Type,
		Name:        def.Name,
		Description: def.Description,
		Severity:    def.BaseSeverity,
		TicksLeft:   1,
		TotalTicks:  def.DurationTicks,
	}

	handleActionLocked(s, p, now, "respond_crisis", "")

	if s.ActiveCrisis != nil {
		t.Fatalf("expected crisis to resolve after response")
	}
	if p.Gold != 10-def.GoldCost {
		t.Fatalf("expected gold reduced by crisis cost, got %d", p.Gold)
	}
	if p.Grain != 4-def.GrainCost {
		t.Fatalf("expected grain reduced by crisis cost, got %d", p.Grain)
	}
	if p.Rep != def.ResolveRepDelta {
		t.Fatalf("expected rep delta %d, got %d", def.ResolveRepDelta, p.Rep)
	}
}

func TestCrisisTickAppliesPressure(t *testing.T) {
	s := newTestStore()
	now := time.Now().UTC()
	def, ok := crisisDefinitionByType("fire")
	if !ok {
		t.Fatalf("expected fire crisis definition")
	}
	s.ActiveCrisis = &Crisis{
		Type:        def.Type,
		Name:        def.Name,
		Description: def.Description,
		Severity:    2,
		TicksLeft:   2,
		TotalTicks:  2,
	}
	startGrain := s.World.GrainSupply
	startUnrest := s.World.UnrestValue

	processCrisisTickLocked(s, now)

	if s.World.GrainSupply >= startGrain {
		t.Fatalf("expected crisis to reduce grain supply")
	}
	if s.World.UnrestValue <= startUnrest {
		t.Fatalf("expected crisis to raise unrest")
	}
	if s.ActiveCrisis == nil || s.ActiveCrisis.TicksLeft != 1 {
		t.Fatalf("expected crisis tick to decrement ticks left")
	}
}

func TestWhisperSuccessIncrementsRumors(t *testing.T) {
	s := newTestStore()
	now := time.Now().UTC()
	p1 := &Player{ID: "p1", Name: "Ash Crow (Guest)", Gold: 20, Rep: 0, LastSeen: now}
	p2 := &Player{ID: "p2", Name: "Bran Vale (Guest)", Gold: 20, Rep: 0, LastSeen: now}
	s.Players[p1.ID] = p1
	s.Players[p2.ID] = p2

	ok := handleChatLocked(s, p1, now, "/w Bran route clear")
	if !ok {
		t.Fatalf("expected whisper to succeed")
	}
	if p1.Rumors != 1 {
		t.Fatalf("successful whisper should grant rumors, got %d", p1.Rumors)
	}

	ok = handleChatLocked(s, p1, now.Add(time.Second), "/w Nobody route clear")
	if ok {
		t.Fatalf("unknown whisper target should fail")
	}
	if p1.Rumors != 1 {
		t.Fatalf("failed whisper should not grant rumors, got %d", p1.Rumors)
	}
}

func TestDeliverRequiresGold(t *testing.T) {
	s := newTestStore()
	now := time.Now().UTC()
	p := &Player{ID: "p1", Name: "Ash Crow (Guest)", Gold: 1, Rep: 0, LastSeen: now}
	s.Players[p.ID] = p
	s.Contracts["c1"] = &Contract{ID: "c1", Type: "Emergency", DeadlineTicks: 3, Status: "Accepted", OwnerPlayerID: p.ID, OwnerName: p.Name}

	handleActionLocked(s, p, now, "deliver", "c1")
	if p.Gold != 1 {
		t.Fatalf("deliver should require gold before attempting, got %d", p.Gold)
	}
	if s.Contracts["c1"].Status != "Accepted" {
		t.Fatalf("deliver should not complete without gold, status=%s", s.Contracts["c1"].Status)
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

func TestObligationOverdueAndSettle(t *testing.T) {
	s := newTestStore()
	now := time.Now().UTC()
	creditor := &Player{ID: "p1", Name: "Ash Crow (Guest)", Gold: 0, Rep: 0, Heat: 0, LastSeen: now}
	debtor := &Player{ID: "p2", Name: "Bran Vale (Guest)", Gold: 20, Rep: 0, Heat: 0, LastSeen: now}
	s.Players[creditor.ID] = creditor
	s.Players[debtor.ID] = debtor

	ob := &Obligation{
		ID:               "o-1",
		CreditorPlayerID: creditor.ID,
		CreditorName:     creditor.Name,
		DebtorPlayerID:   debtor.ID,
		DebtorName:       debtor.Name,
		Reason:           "silence payment",
		Severity:         3,
		DueTick:          s.TickCount,
		Status:           "Open",
	}
	s.Obligations[ob.ID] = ob

	processFinanceTickLocked(s, now)
	if ob.Status != "Overdue" {
		t.Fatalf("expected obligation overdue, got %s", ob.Status)
	}
	if debtor.Rep != -4 || debtor.Heat != 1 {
		t.Fatalf("expected overdue penalties, rep=%d heat=%d", debtor.Rep, debtor.Heat)
	}

	handleActionInputLocked(s, debtor, now, ActionInput{Action: "settle_obligation", ObligationID: ob.ID})
	if ob.Status != "Settled" {
		t.Fatalf("expected obligation settled, got %s", ob.Status)
	}
	if debtor.Gold != 11 || creditor.Gold != 9 {
		t.Fatalf("expected settlement transfer, debtor=%d creditor=%d", debtor.Gold, creditor.Gold)
	}
	if debtor.Rep != -2 || debtor.Heat != 0 {
		t.Fatalf("expected settlement recovery, rep=%d heat=%d", debtor.Rep, debtor.Heat)
	}
}

func TestSupplyContractFlow(t *testing.T) {
	s := newTestStore()
	now := time.Now().UTC()

	issuer := &Player{ID: "p1", Name: "Ash Crow (Guest)", Gold: 30, Rep: 0, Grain: 0, LastSeen: now}
	contractor := &Player{ID: "p2", Name: "Bran Vale (Guest)", Gold: 10, Rep: 0, Grain: 5, LastSeen: now}
	s.Players[issuer.ID] = issuer
	s.Players[contractor.ID] = contractor
	s.World.UnrestValue = 20

	handleActionInputLocked(s, issuer, now, ActionInput{Action: "post_supply", Sacks: 4, Reward: 12})

	var contractID string
	for id, c := range s.Contracts {
		if c.Type == "Supply" {
			contractID = id
		}
	}
	if contractID == "" {
		t.Fatalf("expected supply contract to be created")
	}
	c := s.Contracts[contractID]
	if c.IssuerPlayerID != issuer.ID || c.RewardGold != 12 || c.SupplySacks != 4 {
		t.Fatalf("unexpected supply contract fields: %+v", c)
	}
	if issuer.Gold != 18 {
		t.Fatalf("issuer should escrow reward, gold=%d", issuer.Gold)
	}

	handleActionLocked(s, contractor, now.Add(2*time.Second), "accept", contractID)
	if c.Status != "Accepted" || c.OwnerPlayerID != contractor.ID {
		t.Fatalf("contract should be accepted by contractor, status=%s owner=%s", c.Status, c.OwnerPlayerID)
	}

	prevSupply := s.World.GrainSupply
	handleActionLocked(s, contractor, now.Add(4*time.Second), "deliver", contractID)
	if c.Status != "Completed" {
		t.Fatalf("supply contract should complete, status=%s", c.Status)
	}
	if contractor.Gold != 22 {
		t.Fatalf("contractor should receive reward, gold=%d", contractor.Gold)
	}
	if contractor.Grain != 1 {
		t.Fatalf("contractor grain should be consumed, grain=%d", contractor.Grain)
	}
	if s.World.GrainSupply != prevSupply+4*grainUnitPerSack {
		t.Fatalf("world grain supply should increase, got %d want %d", s.World.GrainSupply, prevSupply+4*grainUnitPerSack)
	}
	if issuer.Rep != 1 {
		t.Fatalf("issuer should gain reputation, rep=%d", issuer.Rep)
	}
}

func TestLaunchProjectCreatesAndCostsResources(t *testing.T) {
	s := newTestStore()
	now := time.Now().UTC()
	p := &Player{ID: "p1", Name: "Ash Crow (Guest)", Gold: 20, Rep: 0, Grain: 6, LastSeen: now}
	s.Players[p.ID] = p

	def, ok := projectDefinitionByType("granary_reinforcement")
	if !ok {
		t.Fatalf("expected granary project definition")
	}

	handleActionInputLocked(s, p, now, ActionInput{Action: "launch_project", ProjectType: def.Type})

	if len(s.Projects) != 1 {
		t.Fatalf("expected one project, got %d", len(s.Projects))
	}
	if p.Gold != 20-def.CostGold {
		t.Fatalf("project should spend gold, got %d want %d", p.Gold, 20-def.CostGold)
	}
	if p.Grain != 6-def.CostGrain {
		t.Fatalf("project should spend grain, got %d want %d", p.Grain, 6-def.CostGrain)
	}
	if s.DailyHighImpactN[p.ID] != 1 {
		t.Fatalf("project should consume high impact, got %d", s.DailyHighImpactN[p.ID])
	}
}

func TestProjectCompletionAppliesEffects(t *testing.T) {
	s := newTestStore()
	now := time.Now().UTC()
	p := &Player{ID: "p1", Name: "Ash Crow (Guest)", Gold: 0, Rep: 0, Heat: 5, Grain: 0, LastSeen: now}
	s.Players[p.ID] = p

	def, ok := projectDefinitionByType("civic_patrols")
	if !ok {
		t.Fatalf("expected civic patrols definition")
	}

	startGrain := s.World.GrainSupply
	s.World.UnrestValue = 20
	s.Projects["p-1"] = &Project{
		ID:            "p-1",
		Type:          def.Type,
		Name:          def.Name,
		OwnerPlayerID: p.ID,
		OwnerName:     p.Name,
		TicksLeft:     1,
		TotalTicks:    def.DurationTicks,
	}

	processProjectTickLocked(s, now)

	if len(s.Projects) != 0 {
		t.Fatalf("project should complete and be removed")
	}
	expectedUnrest := clampInt(20+def.UnrestDelta, 0, 100)
	if s.World.UnrestValue != expectedUnrest {
		t.Fatalf("unexpected unrest value, got %d want %d", s.World.UnrestValue, expectedUnrest)
	}
	expectedHeat := clampInt(5+def.HeatDelta, 0, 20)
	if p.Heat != expectedHeat {
		t.Fatalf("unexpected heat value, got %d want %d", p.Heat, expectedHeat)
	}
	if s.World.GrainSupply != startGrain {
		t.Fatalf("grain supply should remain unchanged, got %d want %d", s.World.GrainSupply, startGrain)
	}
}

func TestWardLanternsProjectStartsWardNetwork(t *testing.T) {
	s := newTestStore()
	now := time.Now().UTC()
	p := &Player{ID: "p1", Name: "Ash Crow (Guest)", Gold: 0, Rep: 0, Heat: 0, Grain: 0, LastSeen: now}
	s.Players[p.ID] = p

	def, ok := projectDefinitionByType("ward_lanterns")
	if !ok {
		t.Fatalf("expected ward lanterns definition")
	}

	s.Projects["p-1"] = &Project{
		ID:            "p-1",
		Type:          def.Type,
		Name:          def.Name,
		OwnerPlayerID: p.ID,
		OwnerName:     p.Name,
		TicksLeft:     1,
		TotalTicks:    def.DurationTicks,
	}

	processProjectTickLocked(s, now)

	if s.World.WardNetworkTicks != def.WardNetworkTicks {
		t.Fatalf("ward network ticks should be set, got %d want %d", s.World.WardNetworkTicks, def.WardNetworkTicks)
	}
}

func TestWardedNetworkDampensRumors(t *testing.T) {
	s := newTestStore()
	now := time.Now().UTC()
	target := &Player{ID: "p1", Name: "Ash Crow (Guest)", Gold: 0, Rep: 0, Heat: 0, LastSeen: now}
	s.Players[target.ID] = target

	s.World.WardNetworkTicks = 2
	s.Rumors[1] = &Rumor{
		ID:             1,
		Claim:          "Whispers swirl",
		Topic:          "corruption",
		TargetPlayerID: target.ID,
		TargetName:     target.Name,
		SourcePlayerID: "p2",
		SourceName:     "Bran Vale (Guest)",
		Credibility:    6,
		Spread:         1,
		Decay:          3,
	}

	processIntelTickLocked(s, now)

	r := s.Rumors[1]
	if r.Spread != 2 {
		t.Fatalf("warded rumor spread should slow, got spread=%d", r.Spread)
	}
	if r.Decay != 1 {
		t.Fatalf("warded rumor decay should accelerate, got decay=%d", r.Decay)
	}
}

func TestBountyDeliveryRequiresEvidence(t *testing.T) {
	s := newTestStore()
	now := time.Now().UTC()
	hunter := &Player{ID: "p1", Name: "Ash Crow (Guest)", Gold: 20, Rep: 0, LastSeen: now}
	target := &Player{ID: "p2", Name: "Bran Vale (Guest)", Gold: 20, Rep: 0, Heat: 12, LastSeen: now}
	s.Players[hunter.ID] = hunter
	s.Players[target.ID] = target

	issueBountyContractLocked(s, target, bountyDeadlineTicks)
	var bountyID string
	for id, c := range s.Contracts {
		if c.Type == "Bounty" {
			bountyID = id
			break
		}
	}
	if bountyID == "" {
		t.Fatalf("expected bounty contract issued")
	}

	handleActionLocked(s, hunter, now, "accept", bountyID)
	handleActionLocked(s, hunter, now, "deliver", bountyID)
	if s.Contracts[bountyID].Status != "Accepted" {
		t.Fatalf("bounty should remain accepted without evidence, got %s", s.Contracts[bountyID].Status)
	}

	addEvidenceLocked(s, hunter, target, "corruption", 6, 5)
	handleActionLocked(s, hunter, now.Add(2*time.Second), "deliver", bountyID)
	if s.Contracts[bountyID].Status != "Completed" {
		t.Fatalf("bounty should complete after evidence delivery, got %s", s.Contracts[bountyID].Status)
	}
	if target.Heat != 7 {
		t.Fatalf("bounty should reduce target heat, got %d", target.Heat)
	}
	if hunter.Gold <= 20 {
		t.Fatalf("bounty should pay hunter, got %d", hunter.Gold)
	}
}

func TestMarketBuySellAndRelief(t *testing.T) {
	s := newTestStore()
	now := time.Now().UTC()
	p := &Player{ID: "p1", Name: "Ash Crow (Guest)", Gold: 20, Rep: 0, LastSeen: now}
	s.Players[p.ID] = p
	s.World.GrainSupply = 120
	s.World.GrainTier = grainTierFromSupply(s.World.GrainSupply)

	base := marketBasePrice(s.World.GrainTier)
	buyPrice := marketBuyPrice(base, s.Policies.TaxRatePct, s.World.RestrictedMarketsTicks)
	sellPrice := marketSellPrice(base, s.Policies.TaxRatePct, s.World.RestrictedMarketsTicks)

	handleActionInputLocked(s, p, now, ActionInput{Action: "buy_grain", Amount: 2})
	if p.Grain != 2 {
		t.Fatalf("expected 2 sacks after buy, got %d", p.Grain)
	}
	if p.Gold != 20-2*buyPrice {
		t.Fatalf("gold after buy = %d, want %d", p.Gold, 20-2*buyPrice)
	}
	if s.World.GrainSupply != 120-2*grainUnitPerSack {
		t.Fatalf("grain supply should drop after buy, got %d", s.World.GrainSupply)
	}

	handleActionInputLocked(s, p, now, ActionInput{Action: "sell_grain", Amount: 1})
	if p.Grain != 1 {
		t.Fatalf("expected 1 sack after sell, got %d", p.Grain)
	}
	if p.Gold != 20-2*buyPrice+sellPrice {
		t.Fatalf("gold after sell = %d, want %d", p.Gold, 20-2*buyPrice+sellPrice)
	}
	if s.World.GrainSupply != 120-2*grainUnitPerSack+grainUnitPerSack {
		t.Fatalf("grain supply should rise after sell, got %d", s.World.GrainSupply)
	}

	p.Grain = reliefSackCost
	s.World.UnrestValue = 20
	prevUnrest := s.World.UnrestValue
	handleActionInputLocked(s, p, now, ActionInput{Action: "donate_relief"})
	if p.Grain != 0 {
		t.Fatalf("relief should consume sacks, got %d", p.Grain)
	}
	if s.World.UnrestValue >= prevUnrest {
		t.Fatalf("relief should reduce unrest, got %d", s.World.UnrestValue)
	}
}

func TestDeliverConsumesRumorAndAppliesBonus(t *testing.T) {
	s := newTestStore()
	now := time.Now().UTC()
	p := &Player{ID: "p1", Name: "Ash Crow (Guest)", Gold: 20, Rep: 0, Rumors: 1, LastSeen: now}
	s.Players[p.ID] = p
	s.Contracts["c1"] = &Contract{ID: "c1", Type: "Emergency", DeadlineTicks: 3, Status: "Fulfilled", OwnerPlayerID: p.ID, OwnerName: p.Name, Stance: contractStanceCareful}

	base := computeDeliverOutcomeLocked(s, &Player{Rep: 0, Rumors: 0}, s.Contracts["c1"])
	withRumor := computeDeliverOutcomeLocked(s, p, s.Contracts["c1"])
	if withRumor.RewardGold != base.RewardGold+rumorDeliverBonusGold {
		t.Fatalf("rumor bonus should add %d gold, base=%d withRumor=%d", rumorDeliverBonusGold, base.RewardGold, withRumor.RewardGold)
	}

	handleActionLocked(s, p, now, "deliver", "c1")
	if p.Rumors != 0 {
		t.Fatalf("successful delivery should consume one rumor, got %d", p.Rumors)
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
	s.Contracts["accepted"] = &Contract{ID: "accepted", Type: "Emergency", DeadlineTicks: 2, Status: "Accepted", OwnerPlayerID: p.ID, OwnerName: p.Name, Stance: contractStanceCareful}
	s.Contracts["fulfilled"] = &Contract{ID: "fulfilled", Type: "Emergency", DeadlineTicks: 1, Status: "Fulfilled", OwnerPlayerID: p.ID, OwnerName: p.Name, Stance: contractStanceCareful}
	s.Contracts["completed"] = &Contract{ID: "completed", Type: "Emergency", DeadlineTicks: 3, Status: "Completed", OwnerPlayerID: p.ID, OwnerName: p.Name}

	data := buildPageDataLocked(s, p.ID, false)
	views := map[string]ContractView{}
	for _, cv := range data.Contracts {
		views[cv.ID] = cv
	}

	if !views["issued"].CanAccept || !views["issued"].CanIgnore || views["issued"].CanDeliver || views["issued"].CanAbandon {
		t.Fatalf("issued contract should show only accept/ignore: %#v", views["issued"])
	}
	if views["issued"].UrgencyClass != "" {
		t.Fatalf("issued contract with deadline=3 should be neutral, got %q", views["issued"].UrgencyClass)
	}
	if !views["accepted"].CanDeliver || !views["accepted"].CanAbandon || views["accepted"].CanAccept || views["accepted"].CanIgnore {
		t.Fatalf("accepted contract should show deliver/abandon: %#v", views["accepted"])
	}
	if views["accepted"].UrgencyClass != "warning" {
		t.Fatalf("accepted contract with deadline=2 should be warning, got %q", views["accepted"].UrgencyClass)
	}
	if views["accepted"].DeliverLabel != "Deliver (+20g)" {
		t.Fatalf("accepted contract deliver label mismatch: %q", views["accepted"].DeliverLabel)
	}
	if !views["fulfilled"].CanDeliver || views["fulfilled"].CanAbandon || views["fulfilled"].CanAccept || views["fulfilled"].CanIgnore {
		t.Fatalf("fulfilled contract should show deliver only: %#v", views["fulfilled"])
	}
	if views["fulfilled"].UrgencyClass != "" {
		t.Fatalf("fulfilled contract should be neutral urgency, got %q", views["fulfilled"].UrgencyClass)
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

func TestContractCapPrioritizesIssuedOverNonActionableFulfilled(t *testing.T) {
	s := newTestStore()
	now := time.Now().UTC()
	p := &Player{ID: "p1", Name: "Ash Crow (Guest)", Gold: 20, Rep: 0, LastSeen: now}
	s.Players[p.ID] = p

	// Fill with non-actionable fulfilled contracts first.
	for i := 1; i <= maxVisibleContracts+3; i++ {
		id := fmt.Sprintf("c-%d", i)
		s.Contracts[id] = &Contract{
			ID:            id,
			Type:          "Emergency",
			DeadlineTicks: 3,
			Status:        "Fulfilled",
			OwnerPlayerID: "other",
			OwnerName:     "Bran Vale (Guest)",
			IssuedAtTick:  int64(i),
		}
	}
	// Add one issued contract that should still be visible.
	s.Contracts["c-999"] = &Contract{
		ID:            "c-999",
		Type:          "Smuggling",
		DeadlineTicks: 2,
		Status:        "Issued",
		IssuedAtTick:  999,
	}

	data := buildPageDataLocked(s, p.ID, false)
	foundIssued := false
	for _, cv := range data.Contracts {
		if cv.ID == "c-999" && cv.Status == "Issued" && cv.CanAccept {
			foundIssued = true
			break
		}
	}
	if !foundIssued {
		t.Fatalf("issued contract should remain visible under cap when non-actionable fulfilled contracts exist")
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
	if got := computeDeliverOutcomeLocked(nil, p, c).RewardGold; got != 22 {
		t.Fatalf("careful reward = %d, want 22", got)
	}
	c.Stance = contractStanceFast
	if got := computeDeliverOutcomeLocked(nil, p, c).RewardGold; got != 27 {
		t.Fatalf("fast reward = %d, want 27", got)
	}
	c.Stance = contractStanceQuiet
	if got := computeDeliverOutcomeLocked(nil, p, c).RewardGold; got != 20 {
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
		nil,
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

func TestSeatElectionChoosesHighestRep(t *testing.T) {
	s := newTestStore()
	now := time.Now().UTC()
	low := &Player{ID: "p1", Name: "Ash Crow (Guest)", Rep: 5, LastSeen: now}
	high := &Player{ID: "p2", Name: "Bran Vale (Guest)", Rep: 30, LastSeen: now}
	s.Players[low.ID] = low
	s.Players[high.ID] = high
	s.Seats["master_of_coin"].TenureTicksLeft = 1

	runWorldTickLocked(s, now)
	if s.Seats["master_of_coin"].ElectionWindowTicks == 0 {
		t.Fatalf("expected election window to open after tenure expiry")
	}

	runWorldTickLocked(s, now.Add(time.Minute))
	runWorldTickLocked(s, now.Add(2*time.Minute))
	if got := s.Seats["master_of_coin"].HolderPlayerID; got != high.ID {
		t.Fatalf("expected highest rep player to win election, got holder=%q", got)
	}
}

func TestTaxPolicyChangesDeliveryOutcome(t *testing.T) {
	s := newTestStore()
	now := time.Now().UTC()
	p := &Player{ID: "p1", Name: "Ash Crow (Guest)", Gold: 20, Rep: 0, LastSeen: now}
	s.Players[p.ID] = p
	s.Seats["master_of_coin"].HolderPlayerID = p.ID
	s.Seats["master_of_coin"].HolderName = p.Name
	c := &Contract{ID: "c1", Type: "Emergency", Stance: contractStanceCareful}

	base := computeDeliverOutcomeLocked(s, p, c).RewardGold
	handleActionLocked(s, p, now, "set_tax_high", "")
	if s.Policies.TaxRatePct != 20 {
		t.Fatalf("expected high tax to be applied")
	}
	afterTax := computeDeliverOutcomeLocked(s, p, c).RewardGold
	if afterTax >= base {
		t.Fatalf("expected high tax to reduce reward, base=%d after=%d", base, afterTax)
	}
}

func TestEmbargoBlocksSmugglingAcceptUnlessHarborMaster(t *testing.T) {
	s := newTestStore()
	now := time.Now().UTC()
	p := &Player{ID: "p1", Name: "Ash Crow (Guest)", Gold: 20, Rep: 0, LastSeen: now}
	s.Players[p.ID] = p
	s.Contracts["c1"] = &Contract{ID: "c1", Type: "Smuggling", DeadlineTicks: 3, Status: "Issued"}
	s.Policies.SmugglingEmbargoTicks = 2

	handleActionLocked(s, p, now, "accept", "c1")
	if s.Contracts["c1"].Status != "Issued" {
		t.Fatalf("smuggling should be blocked under embargo for non-holder")
	}

	s.Seats["harbor_master"].HolderPlayerID = p.ID
	s.Seats["harbor_master"].HolderName = p.Name
	handleActionLocked(s, p, now.Add(time.Second), "accept", "c1")
	if s.Contracts["c1"].Status != "Accepted" {
		t.Fatalf("harbor master should be able to bypass embargo")
	}
}

func TestScenarioInformationAttackInstitutionResponseEconomicConsequence(t *testing.T) {
	s := newTestStore()
	now := time.Now().UTC()
	attacker := &Player{ID: "p1", Name: "Ash Crow (Guest)", Gold: 20, Rep: 15, LastSeen: now}
	target := &Player{ID: "p2", Name: "Bran Vale (Guest)", Gold: 20, Rep: 5, LastSeen: now}
	s.Players[attacker.ID] = attacker
	s.Players[target.ID] = target
	s.Seats["harbor_master"].HolderPlayerID = target.ID
	s.Seats["harbor_master"].HolderName = target.Name
	s.Contracts["c1"] = &Contract{ID: "c1", Type: "Emergency", DeadlineTicks: 3, Status: "Issued"}

	addEvidenceLocked(s, attacker, target, "corruption", 8, 5)
	handleActionInputLocked(s, attacker, now, ActionInput{Action: "publish_evidence", TargetID: target.ID})

	if !s.Policies.PermitRequiredHighRisk {
		t.Fatalf("expected evidence publication to trigger permit policy sanction")
	}
	if s.Seats["harbor_master"].HolderPlayerID != "" {
		t.Fatalf("sanction should remove target from harbor master seat")
	}

	handleActionLocked(s, target, now.Add(time.Second), "accept", "c1")
	if s.Contracts["c1"].Status != "Issued" {
		t.Fatalf("permit sanction should block target's emergency contract accept")
	}
}

func TestScenarioCreditDefaultSanctionEmbargoSmugglingResponse(t *testing.T) {
	s := newTestStore()
	now := time.Now().UTC()
	lender := &Player{ID: "p1", Name: "Ash Crow (Guest)", Gold: 50, Rep: 10, LastSeen: now}
	borrower := &Player{ID: "p2", Name: "Bran Vale (Guest)", Gold: 1, Rep: 0, LastSeen: now}
	smuggler := &Player{ID: "p3", Name: "Corin Reed (Guest)", Gold: 20, Rep: 0, LastSeen: now}
	s.Players[lender.ID] = lender
	s.Players[borrower.ID] = borrower
	s.Players[smuggler.ID] = smuggler

	handleActionInputLocked(s, lender, now, ActionInput{Action: "loan_offer", TargetID: borrower.ID, Amount: 10})
	var loanID string
	for id := range s.Loans {
		loanID = id
	}
	if loanID == "" {
		t.Fatalf("expected loan offer to be created")
	}
	handleActionInputLocked(s, borrower, now.Add(time.Second), ActionInput{Action: "loan_accept", LoanID: loanID})
	handleActionInputLocked(s, borrower, now.Add(2*time.Second), ActionInput{Action: "default", LoanID: loanID})

	if s.Policies.SmugglingEmbargoTicks <= 0 {
		t.Fatalf("default should trigger embargo/sanction window")
	}

	s.Contracts["c1"] = &Contract{ID: "c1", Type: "Smuggling", DeadlineTicks: 3, Status: "Issued"}
	handleActionLocked(s, borrower, now.Add(3*time.Second), "accept", "c1")
	if s.Contracts["c1"].Status != "Issued" {
		t.Fatalf("borrower should be blocked by embargo from smuggling accept")
	}

	s.Seats["harbor_master"].HolderPlayerID = smuggler.ID
	s.Seats["harbor_master"].HolderName = smuggler.Name
	handleActionLocked(s, smuggler, now.Add(4*time.Second), "accept", "c1")
	if s.Contracts["c1"].Status != "Accepted" {
		t.Fatalf("harbor master should perform smuggling response under embargo")
	}

	withEmbargo := computeDeliverOutcomeLocked(s, smuggler, s.Contracts["c1"]).RewardGold
	s.Policies.SmugglingEmbargoTicks = 0
	noEmbargo := computeDeliverOutcomeLocked(s, smuggler, s.Contracts["c1"]).RewardGold
	if withEmbargo <= noEmbargo {
		t.Fatalf("embargo should increase smuggling value response, with=%d without=%d", withEmbargo, noEmbargo)
	}
}

func TestTravelProgressAndArrival(t *testing.T) {
	s := newTestStore()
	now := time.Now().UTC()
	p := &Player{ID: "p1", Name: "Ash Crow (Guest)", Gold: 10, Rep: 0, LastSeen: now, LocationID: locationCapital}
	s.Players[p.ID] = p

	handleActionInputLocked(s, p, now, ActionInput{Action: "travel", LocationID: locationFrontier})

	wantTicks := travelTicksBetween(locationCapital, locationFrontier)
	if p.TravelTicksLeft != wantTicks {
		t.Fatalf("travel ticks = %d, want %d", p.TravelTicksLeft, wantTicks)
	}
	if p.LocationID != locationCapital {
		t.Fatalf("location should remain capital during travel, got %s", p.LocationID)
	}

	processTravelTickLocked(s, now)
	if p.TravelTicksLeft != wantTicks-1 {
		t.Fatalf("travel ticks after one tick = %d, want %d", p.TravelTicksLeft, wantTicks-1)
	}

	processTravelTickLocked(s, now)
	if p.LocationID != locationFrontier {
		t.Fatalf("expected arrival at frontier, got %s", p.LocationID)
	}
	if p.TravelTicksLeft != 0 || p.TravelToID != "" {
		t.Fatalf("travel state should clear on arrival, left=%d to=%s", p.TravelTicksLeft, p.TravelToID)
	}
}

func TestTravelBlocksActions(t *testing.T) {
	s := newTestStore()
	now := time.Now().UTC()
	p := &Player{ID: "p1", Name: "Ash Crow (Guest)", Gold: 10, Rep: 0, LastSeen: now, LocationID: locationCapital}
	s.Players[p.ID] = p
	s.World.UnrestValue = 20

	handleActionInputLocked(s, p, now, ActionInput{Action: "travel", LocationID: locationFrontier})
	handleActionLocked(s, p, now, "investigate", "")

	if s.World.UnrestValue != 20 {
		t.Fatalf("investigate should be blocked while traveling, unrest=%d", s.World.UnrestValue)
	}
	if s.ToastByPlayer[p.ID] == "" {
		t.Fatalf("expected toast when acting during travel")
	}
}

func TestExploreRuinsAppliesOutcomeAndCooldown(t *testing.T) {
	s := newTestStore()
	now := time.Now().UTC()
	p := &Player{ID: "p1", Name: "Ash Crow (Guest)", Gold: 5, Rep: 0, Heat: 0, LastSeen: now, LocationID: locationRuins}
	s.Players[p.ID] = p

	handleActionInputLocked(s, p, now, ActionInput{Action: "explore_ruins"})

	if p.Gold != 4 {
		t.Fatalf("expected supplies cost applied, gold=%d", p.Gold)
	}
	if p.Heat != 2 || p.Rep != -2 {
		t.Fatalf("expected ruins mishap outcome, heat=%d rep=%d", p.Heat, p.Rep)
	}
	if _, ok := s.LastFieldworkAt[p.ID]; !ok {
		t.Fatalf("expected fieldwork cooldown set")
	}
}
