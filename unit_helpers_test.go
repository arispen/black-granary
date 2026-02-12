package main

import (
	mathrand "math/rand"
	"testing"
	"time"
)

func TestNarrativeHelpers(t *testing.T) {
	if got := grainTierNarrative("Stable", "Stable"); got != "" {
		t.Fatalf("grainTierNarrative unchanged = %q, want empty", got)
	}
	if got := grainTierNarrative("Stable", "Tight"); got != "Grain supply tightens." {
		t.Fatalf("grainTierNarrative Tight = %q", got)
	}
	if got := grainTierNarrative("Tight", "Scarce"); got != "Grain stores thin across the city." {
		t.Fatalf("grainTierNarrative Scarce = %q", got)
	}
	if got := grainTierNarrative("Scarce", "Critical"); got != "Grain stores fall below emergency reserves." {
		t.Fatalf("grainTierNarrative Critical = %q", got)
	}
	if got := grainTierNarrative("Critical", "Stable"); got != "Fresh grain reaches the markets, easing shortages." {
		t.Fatalf("grainTierNarrative easing = %q", got)
	}

	if got := unrestTierNarrative("Calm", "Calm"); got != "" {
		t.Fatalf("unrestTierNarrative unchanged = %q, want empty", got)
	}
	if got := unrestTierNarrative("Calm", "Uneasy"); got != "Whispers of worry spread through the streets." {
		t.Fatalf("unrestTierNarrative Uneasy = %q", got)
	}
	if got := unrestTierNarrative("Uneasy", "Unstable"); got != "Tension rises as crowds gather and tempers flare." {
		t.Fatalf("unrestTierNarrative Unstable = %q", got)
	}
	if got := unrestTierNarrative("Unstable", "Rioting"); got != "The city erupts into open unrest." {
		t.Fatalf("unrestTierNarrative Rioting = %q", got)
	}
	if got := unrestTierNarrative("Rioting", "Calm"); got != "The streets quiet as tensions ease." {
		t.Fatalf("unrestTierNarrative easing = %q", got)
	}
}

func TestDeriveSituationBranches(t *testing.T) {
	tests := []struct {
		grain  string
		unrest string
		want   string
	}{
		{"Stable", "Calm", "The city breathes-uneasy peace holds."},
		{"Scarce", "Uneasy", "Shortages spread quiet panic through the markets."},
		{"Critical", "Unstable", "Hunger sharpens into anger; deals turn desperate."},
		{"Critical", "Rioting", "The streets burn with desperation and blame."},
		{"Stable", "Rioting", "Fires, fear, and blame race faster than grain."},
		{"Critical", "Calm", "Emergency stores fray as every convoy is contested."},
		{"Scarce", "Calm", "Every sack matters and every alley has a price."},
		{"Tight", "Unstable", "Crowds watch each cart as trust thins by the hour."},
		{"Tight", "Uneasy", "Merchants bargain in low voices while the city waits."},
	}

	for _, tc := range tests {
		if got := deriveSituation(tc.grain, tc.unrest); got != tc.want {
			t.Fatalf("deriveSituation(%q, %q) = %q, want %q", tc.grain, tc.unrest, got, tc.want)
		}
	}
}

func TestDeliverChanceByTierAndMaxFloat(t *testing.T) {
	if got := deliverChanceByTier("Stable"); got != 80 {
		t.Fatalf("deliverChanceByTier(Stable) = %d", got)
	}
	if got := deliverChanceByTier("Tight"); got != 65 {
		t.Fatalf("deliverChanceByTier(Tight) = %d", got)
	}
	if got := deliverChanceByTier("Scarce"); got != 50 {
		t.Fatalf("deliverChanceByTier(Scarce) = %d", got)
	}
	if got := deliverChanceByTier("unknown"); got != 35 {
		t.Fatalf("deliverChanceByTier(default) = %d", got)
	}
	if got := maxFloat(1.25, 4.5); got != 4.5 {
		t.Fatalf("maxFloat(1.25,4.5) = %v", got)
	}
	if got := maxFloat(4.5, 1.25); got != 4.5 {
		t.Fatalf("maxFloat(4.5,1.25) = %v", got)
	}
}

func TestRelicHelpers(t *testing.T) {
	defs := relicDefinitions()
	if len(defs) == 0 {
		t.Fatalf("relicDefinitions should not be empty")
	}

	seen := make(map[string]bool, len(defs))
	for _, d := range defs {
		seen[d.Name+"|"+d.Effect] = true
	}

	rng := mathrand.New(mathrand.NewSource(1))
	got := randomRelicDefinition(rng)
	if !seen[got.Name+"|"+got.Effect] {
		t.Fatalf("randomRelicDefinition returned unknown relic: %+v", got)
	}

	if label := relicEffectLabel(nil); label != "" {
		t.Fatalf("relicEffectLabel(nil) = %q, want empty", label)
	}
	if label := relicEffectLabel(&Relic{Effect: "heat", Power: 2}); label != "Effect: -2 heat" {
		t.Fatalf("heat label = %q", label)
	}
	if label := relicEffectLabel(&Relic{Effect: "rep", Power: 3}); label != "Effect: +3 reputation" {
		t.Fatalf("rep label = %q", label)
	}
	if label := relicEffectLabel(&Relic{Effect: "gold", Power: 4}); label != "Effect: +4g" {
		t.Fatalf("gold label = %q", label)
	}
	if label := relicEffectLabel(&Relic{Effect: "rumor", Power: 1}); label != "Effect: +1 rumors" {
		t.Fatalf("rumor label = %q", label)
	}
	if label := relicEffectLabel(&Relic{Effect: "grain", Power: 5}); label != "Effect: +5 sacks" {
		t.Fatalf("grain label = %q", label)
	}
	if label := relicEffectLabel(&Relic{Effect: "mystery", Power: 9}); label != "Effect: unknown" {
		t.Fatalf("unknown label = %q", label)
	}
}

func TestContractAndObligationHelperFunctions(t *testing.T) {
	now := time.Now().UTC()
	s := newTestStore()
	creditor := &Player{ID: "p1", Name: "Creditor", LastSeen: now}
	debtor := &Player{ID: "p2", Name: "Debtor", LastSeen: now}
	s.Players[creditor.ID] = creditor
	s.Players[debtor.ID] = debtor
	s.TickCount = 7

	addObligationLocked(s, creditor, debtor, "test debt", 99)
	ob := s.Obligations["o-1"]
	if ob == nil {
		t.Fatalf("expected obligation to be created")
	}
	if ob.Severity != 5 {
		t.Fatalf("severity should clamp to 5, got %d", ob.Severity)
	}
	if ob.DueTick != s.TickCount+obligationDueTicks {
		t.Fatalf("due tick = %d", ob.DueTick)
	}
	if ob.Status != "Open" {
		t.Fatalf("status = %q", ob.Status)
	}

	contractor := &Player{ID: "p3", Name: "Carrier", Gold: 0, Rep: 0, LastSeen: now}
	s.Players[contractor.ID] = contractor
	applyFulfillmentRewardsLocked(s, &Contract{OwnerPlayerID: contractor.ID, Type: "Emergency"})
	if contractor.Gold != 25 || contractor.Rep != 8 {
		t.Fatalf("emergency rewards incorrect, gold=%d rep=%d", contractor.Gold, contractor.Rep)
	}

	smuggler := &Player{ID: "p4", Name: "Smuggler", Gold: 0, Rep: 0, LastSeen: now}
	s.Players[smuggler.ID] = smuggler
	applyFulfillmentRewardsLocked(s, &Contract{OwnerPlayerID: smuggler.ID, Type: "Smuggling"})
	if smuggler.Gold != 35 || smuggler.Rep != 3 {
		t.Fatalf("smuggling rewards incorrect, gold=%d rep=%d", smuggler.Gold, smuggler.Rep)
	}

	penalized := &Player{ID: "p5", Name: "Penalized", Rep: -95, LastSeen: now}
	applyFailurePenaltyLocked(s, penalized)
	if penalized.Rep != -100 {
		t.Fatalf("failure penalty should clamp to -100, got %d", penalized.Rep)
	}

	heat := &Player{ID: "p6", Name: "Heat", Heat: 2, LastSeen: now}
	adjustHeatForDeliveryLocked(heat, "Emergency")
	if heat.Heat != 2 {
		t.Fatalf("non-smuggling heat changed unexpectedly: %d", heat.Heat)
	}
	adjustHeatForDeliveryLocked(heat, "Smuggling")
	if heat.Heat != 3 {
		t.Fatalf("smuggling should raise heat to 3, got %d", heat.Heat)
	}
}

func TestDBAndCleanupHelpers(t *testing.T) {
	repoSQLite := &SQLRepository{dialect: dialectSQLite}
	repoPostgres := &SQLRepository{dialect: dialectPostgres}
	if got := repoSQLite.bind(2); got != "?" {
		t.Fatalf("sqlite bind = %q", got)
	}
	if got := repoPostgres.bind(2); got != "$2" {
		t.Fatalf("postgres bind = %q", got)
	}
	if got := repoSQLite.insertQuery("items", []string{"a", "b"}); got != "INSERT INTO items (a, b) VALUES (?, ?)" {
		t.Fatalf("sqlite insertQuery = %q", got)
	}
	if got := repoPostgres.insertQuery("items", []string{"a", "b"}); got != "INSERT INTO items (a, b) VALUES ($1, $2)" {
		t.Fatalf("postgres insertQuery = %q", got)
	}
	if got := asJSON(struct {
		A int `json:"a"`
	}{A: 3}); got != "{\"a\":3}" {
		t.Fatalf("asJSON = %q", got)
	}
	if got := nullableTime(time.Time{}); got != nil {
		t.Fatalf("nullableTime(zero) should be nil")
	}
	now := time.Now().UTC()
	if got := nullableTime(now); got != now {
		t.Fatalf("nullableTime(non-zero) mismatch")
	}
	if got := shortID("abcdefghi"); got != "abcdef" {
		t.Fatalf("shortID long = %q", got)
	}
	if got := shortID("abc"); got != "abc" {
		t.Fatalf("shortID short = %q", got)
	}

	s := &Store{}
	ensureRuntimeMaps(s)
	if s.LastChatAt == nil || s.LastMessageAt == nil || s.LastActionAt == nil || s.LastDeliverAt == nil {
		t.Fatalf("time maps should be initialized")
	}
	if s.LastInvestigateAt == nil || s.LastSeatActionAt == nil || s.LastIntelActionAt == nil || s.LastFieldworkAt == nil {
		t.Fatalf("tick maps should be initialized")
	}
	if s.DailyActionDate == nil || s.DailyHighImpactN == nil {
		t.Fatalf("daily maps should be initialized")
	}

	store := newTestStore()
	store.ToastByPlayer["gone"] = "stale"
	now = time.Date(2026, 2, 12, 12, 0, 0, 0, time.UTC)
	store.Events = []Event{
		{Text: "old", At: now.Add(-15 * 24 * time.Hour)},
		{Text: "new", At: now.Add(-2 * 24 * time.Hour)},
	}
	store.Chat = []ChatMessage{
		{Text: "old", At: now.Add(-8 * 24 * time.Hour)},
		{Text: "new", At: now.Add(-1 * time.Hour)},
	}
	store.Messages = []DiplomaticMessage{
		{Subject: "old", At: now.Add(-31 * 24 * time.Hour)},
		{Subject: "new", At: now.Add(-2 * 24 * time.Hour)},
	}
	store.Loans["loan_old"] = &Loan{ID: "loan_old", Status: "Repaid", TerminalAt: now.Add(-31 * 24 * time.Hour)}
	store.Loans["loan_keep"] = &Loan{ID: "loan_keep", Status: "Open", TerminalAt: now.Add(-31 * 24 * time.Hour)}
	store.Obligations["ob_old"] = &Obligation{ID: "ob_old", Status: "Settled", TerminalAt: now.Add(-31 * 24 * time.Hour)}
	store.Obligations["ob_keep"] = &Obligation{ID: "ob_keep", Status: "Overdue", TerminalAt: now.Add(-59 * 24 * time.Hour)}
	store.Players["soft"] = &Player{ID: "soft", Name: "Soft", LastSeen: now.Add(-100 * 24 * time.Hour)}
	store.Players["gone"] = &Player{ID: "gone", Name: "Gone", LastSeen: now.Add(-181 * 24 * time.Hour), Gold: 20, Grain: 5, Rep: 3, Heat: 2, Rumors: 1}

	runDailyCleanupLocked(store, now)

	if len(store.Events) != 1 || store.Events[0].Text != "new" {
		t.Fatalf("events cleanup failed: %+v", store.Events)
	}
	if len(store.Chat) != 1 || store.Chat[0].Text != "new" {
		t.Fatalf("chat cleanup failed: %+v", store.Chat)
	}
	if len(store.Messages) != 1 || store.Messages[0].Subject != "new" {
		t.Fatalf("messages cleanup failed: %+v", store.Messages)
	}
	if _, ok := store.Loans["loan_old"]; ok {
		t.Fatalf("expected old terminal loan to be removed")
	}
	if _, ok := store.Loans["loan_keep"]; !ok {
		t.Fatalf("expected non-terminal loan to remain")
	}
	if _, ok := store.Obligations["ob_old"]; ok {
		t.Fatalf("expected old settled obligation to be removed")
	}
	if _, ok := store.Obligations["ob_keep"]; !ok {
		t.Fatalf("expected recent overdue obligation to remain")
	}
	if store.Players["soft"].SoftDeletedAt.IsZero() {
		t.Fatalf("expected soft-delete timestamp")
	}
	gone := store.Players["gone"]
	if gone.HardDeletedAt.IsZero() {
		t.Fatalf("expected hard-delete timestamp")
	}
	if gone.Name == "Gone" || gone.Gold != 0 || gone.Grain != 0 || gone.Rep != 0 || gone.Heat != 0 || gone.Rumors != 0 {
		t.Fatalf("expected hard-delete redaction, got %+v", gone)
	}
	if _, ok := store.ToastByPlayer["gone"]; ok {
		t.Fatalf("expected toast cleanup for hard-deleted player")
	}
}
