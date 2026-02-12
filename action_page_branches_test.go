package main

import (
	"strings"
	"testing"
	"time"
)

func TestBuildPageDataToastAndTravelingGuards(t *testing.T) {
	s := newTestStore()
	now := time.Now().UTC()
	p := &Player{
		ID:             "p1",
		Name:           "Ash Crow (Guest)",
		Gold:           1,
		Rep:            -60,
		LastSeen:       now,
		LocationID:     locationCapital,
		TravelToID:     locationFrontier,
		TravelTicksLeft: 2,
	}
	s.Players[p.ID] = p
	s.TickEvery = 60 * time.Second
	s.LastTickAt = now
	s.LastDeliverAt[p.ID] = now
	s.ToastByPlayer[p.ID] = "hello toast"
	s.DailyActionDate[p.ID] = now.Format("2006-01-02")
	s.DailyHighImpactN[p.ID] = highImpactDailyCap + 5

	// Owned accepted contract should show delivery info and disabled state.
	s.Contracts["own-accepted"] = &Contract{
		ID:            "own-accepted",
		Type:          "Emergency",
		Status:        "Accepted",
		OwnerPlayerID: p.ID,
		OwnerName:     p.Name,
		Stance:        contractStanceCareful,
		DeadlineTicks: 1,
	}
	// Smuggling issued contracts should be hidden at very low reputation.
	s.Contracts["smug-issued"] = &Contract{ID: "smug-issued", Type: "Smuggling", Status: "Issued", DeadlineTicks: 2}

	dataPeek := buildPageDataLocked(s, p.ID, false)
	if dataPeek.Toast != "hello toast" {
		t.Fatalf("peek toast mismatch: %q", dataPeek.Toast)
	}
	if dataPeek.HighImpactRemaining != 0 {
		t.Fatalf("high impact remaining should clamp to 0, got %d", dataPeek.HighImpactRemaining)
	}
	if !dataPeek.Traveling {
		t.Fatalf("expected player to be marked as traveling")
	}
	if !dataPeek.ForgeEvidenceDisabled || dataPeek.ForgeEvidenceReason != "Traveling." {
		t.Fatalf("forge evidence should be disabled while traveling, got disabled=%v reason=%q", dataPeek.ForgeEvidenceDisabled, dataPeek.ForgeEvidenceReason)
	}
	for _, opt := range dataPeek.LocationOptions {
		if !opt.Disabled || opt.Reason != "Travel in progress." {
			t.Fatalf("location options should be disabled while traveling: %+v", opt)
		}
	}

	views := map[string]ContractView{}
	for _, cv := range dataPeek.Contracts {
		views[cv.ID] = cv
	}
	if _, ok := views["smug-issued"]; ok {
		t.Fatalf("smuggling issued contract should be hidden for rep < -50")
	}
	own := views["own-accepted"]
	if !own.CanDeliver || !own.DeliverDisabled {
		t.Fatalf("owned accepted contract should be deliverable but disabled, view=%+v", own)
	}
	if !strings.Contains(own.OutcomeNote, "Need 2g") {
		t.Fatalf("expected outcome note to include gold requirement, got %q", own.OutcomeNote)
	}
	if !strings.Contains(own.OutcomeNote, "Delivery cooldown") {
		t.Fatalf("expected outcome note to include cooldown, got %q", own.OutcomeNote)
	}

	dataPop := buildPageDataLocked(s, p.ID, true)
	if dataPop.Toast != "hello toast" {
		t.Fatalf("pop toast mismatch: %q", dataPop.Toast)
	}
	dataAfter := buildPageDataLocked(s, p.ID, false)
	if dataAfter.Toast != "" {
		t.Fatalf("toast should be consumed, got %q", dataAfter.Toast)
	}
}

func TestBuildPageDataContractRequirementNotes(t *testing.T) {
	s := newTestStore()
	now := time.Now().UTC()
	p := &Player{ID: "p1", Name: "Ash Crow (Guest)", Gold: 20, Grain: 1, Rep: 0, LastSeen: now, LocationID: locationCapital}
	target := &Player{ID: "p2", Name: "Bran Vale (Guest)", Gold: 20, Rep: 0, LastSeen: now}
	s.Players[p.ID] = p
	s.Players[target.ID] = target
	s.Policies.PermitRequiredHighRisk = true
	s.Policies.SmugglingEmbargoTicks = 2

	s.Contracts["emergency"] = &Contract{ID: "emergency", Type: "Emergency", Status: "Issued", DeadlineTicks: 2}
	s.Contracts["smuggling"] = &Contract{ID: "smuggling", Type: "Smuggling", Status: "Issued", DeadlineTicks: 2}
	s.Contracts["bounty"] = &Contract{
		ID:             "bounty",
		Type:           "Bounty",
		Status:         "Accepted",
		OwnerPlayerID:  p.ID,
		OwnerName:      p.Name,
		TargetPlayerID: target.ID,
		TargetName:     target.Name,
		BountyReward:   30,
		BountyEvidence: 5,
		Warranted:      true,
	}
	s.Contracts["supply"] = &Contract{
		ID:            "supply",
		Type:          "Supply",
		Status:        "Accepted",
		OwnerPlayerID: p.ID,
		OwnerName:     p.Name,
		SupplySacks:   3,
		RewardGold:    11,
	}

	data := buildPageDataLocked(s, p.ID, false)
	views := map[string]ContractView{}
	for _, cv := range data.Contracts {
		views[cv.ID] = cv
	}

	em := views["emergency"]
	if em.CanAccept {
		t.Fatalf("permit policy should block emergency acceptance without seat for rep<20")
	}
	if !strings.Contains(em.RequirementNote, "permit required") {
		t.Fatalf("expected permit requirement note, got %q", em.RequirementNote)
	}

	sm := views["smuggling"]
	if sm.CanAccept {
		t.Fatalf("embargo should block smuggling acceptance")
	}
	if !strings.Contains(sm.RequirementNote, "smuggling embargo active") {
		t.Fatalf("expected embargo requirement note, got %q", sm.RequirementNote)
	}

	b := views["bounty"]
	if !b.CanDeliver || !b.DeliverDisabled {
		t.Fatalf("bounty should be deliverable but disabled without evidence, view=%+v", b)
	}
	if !strings.Contains(b.OutcomeNote, "Need evidence strength 5+") {
		t.Fatalf("expected bounty evidence note, got %q", b.OutcomeNote)
	}
	if !strings.Contains(b.RequirementNote, "Requirement: evidence strength 5+") {
		t.Fatalf("expected bounty requirement note, got %q", b.RequirementNote)
	}
	if !strings.Contains(b.RewardNote, "30g") {
		t.Fatalf("expected bounty reward note, got %q", b.RewardNote)
	}

	sup := views["supply"]
	if !sup.CanDeliver || !sup.DeliverDisabled {
		t.Fatalf("supply should be deliverable but disabled when grain is low, view=%+v", sup)
	}
	if !strings.Contains(sup.OutcomeNote, "Need 3 sacks") {
		t.Fatalf("expected supply grain note, got %q", sup.OutcomeNote)
	}
	if !strings.Contains(sup.RequirementNote, "deliver 3 sacks") {
		t.Fatalf("expected supply requirement note, got %q", sup.RequirementNote)
	}
	if !strings.Contains(sup.RewardNote, "11g escrowed") {
		t.Fatalf("expected supply reward note, got %q", sup.RewardNote)
	}
}

func TestHandleActionInputValidationAndCancelRefund(t *testing.T) {
	s := newTestStore()
	now := time.Now().UTC()
	issuer := &Player{ID: "p1", Name: "Issuer", Gold: 50, LastSeen: now}
	other := &Player{ID: "p2", Name: "Other", Gold: 5, LastSeen: now}
	s.Players[issuer.ID] = issuer
	s.Players[other.ID] = other

	// Cannot accept own issued contract.
	s.Contracts["own"] = &Contract{ID: "own", Type: "Emergency", Status: "Issued", IssuerPlayerID: issuer.ID, IssuerName: issuer.Name}
	handleActionInputLocked(s, issuer, now, ActionInput{Action: "accept", ContractID: "own"})
	if s.Contracts["own"].Status != "Issued" {
		t.Fatalf("issuer should not be able to accept own contract")
	}
	if !strings.Contains(s.ToastByPlayer[issuer.ID], "cannot accept your own") {
		t.Fatalf("expected own-contract toast, got %q", s.ToastByPlayer[issuer.ID])
	}

	// Bounty target cannot accept own bounty.
	s.Contracts["b1"] = &Contract{ID: "b1", Type: "Bounty", Status: "Issued", TargetPlayerID: other.ID, TargetName: other.Name}
	handleActionInputLocked(s, other, now, ActionInput{Action: "accept", ContractID: "b1"})
	if s.Contracts["b1"].Status != "Issued" {
		t.Fatalf("bounty target should not be able to accept own bounty")
	}

	// Supply post validation branches.
	handleActionInputLocked(s, issuer, now, ActionInput{Action: "post_supply", Sacks: 0, Reward: 10})
	if !strings.Contains(s.ToastByPlayer[issuer.ID], "valid sack count") {
		t.Fatalf("expected invalid sacks toast, got %q", s.ToastByPlayer[issuer.ID])
	}
	handleActionInputLocked(s, issuer, now, ActionInput{Action: "post_supply", Sacks: 3, Reward: 0})
	if !strings.Contains(s.ToastByPlayer[issuer.ID], "valid reward") {
		t.Fatalf("expected invalid reward toast, got %q", s.ToastByPlayer[issuer.ID])
	}
	issuer.Gold = 2
	handleActionInputLocked(s, issuer, now, ActionInput{Action: "post_supply", Sacks: 3, Reward: 10})
	if !strings.Contains(s.ToastByPlayer[issuer.ID], "Insufficient gold") {
		t.Fatalf("expected insufficient gold toast, got %q", s.ToastByPlayer[issuer.ID])
	}

	// Valid post + issuer cancel should refund escrow.
	issuer.Gold = 20
	handleActionInputLocked(s, issuer, now, ActionInput{Action: "post_supply", Sacks: 4, Reward: 9})
	var posted *Contract
	for _, c := range s.Contracts {
		if c.Type == "Supply" && c.IssuerPlayerID == issuer.ID && c.Status == "Issued" {
			posted = c
			break
		}
	}
	if posted == nil {
		t.Fatalf("expected supply contract to be posted")
	}
	goldAfterPost := issuer.Gold
	handleActionInputLocked(s, issuer, now, ActionInput{Action: "cancel_contract", ContractID: posted.ID})
	if posted.Status != "Cancelled" {
		t.Fatalf("expected posted contract to be cancelled, got %s", posted.Status)
	}
	if issuer.Gold != goldAfterPost+posted.RewardGold {
		t.Fatalf("expected escrow refund on cancel, got gold=%d", issuer.Gold)
	}
}
