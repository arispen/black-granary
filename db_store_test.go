package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOpenRepositoryFromEnvErrors(t *testing.T) {
	t.Setenv("DB_SQLITE_PATH", "")
	t.Setenv("DB_POSTGRES_DSN", "")
	t.Setenv("DATABASE_URL", "")

	t.Setenv("DB_DIALECT", "postgres")
	repo, err := openRepositoryFromEnv()
	if err == nil || !strings.Contains(err.Error(), "requires DB_POSTGRES_DSN or DATABASE_URL") {
		t.Fatalf("expected postgres DSN error, got repo=%v err=%v", repo, err)
	}

	t.Setenv("DB_DIALECT", "bogus")
	repo, err = openRepositoryFromEnv()
	if err == nil || !strings.Contains(err.Error(), "unsupported DB_DIALECT") {
		t.Fatalf("expected unsupported dialect error, got repo=%v err=%v", repo, err)
	}
}

func TestRepositorySQLiteRoundTrip(t *testing.T) {
	t.Setenv("DB_DIALECT", "sqlite")
	dbPath := filepath.Join(t.TempDir(), "state.sqlite")
	t.Setenv("DB_SQLITE_PATH", dbPath)

	repo, err := openRepositoryFromEnv()
	if err != nil {
		t.Fatalf("openRepositoryFromEnv sqlite error: %v", err)
	}
	defer repo.db.Close()

	now := time.Date(2026, 2, 12, 12, 0, 0, 0, time.UTC)
	s1 := newStore()
	s1.World.DayNumber = 9
	s1.World.Subphase = "Noon"
	s1.World.GrainSupply = 123
	s1.World.GrainTier = grainTierFromSupply(s1.World.GrainSupply)
	s1.World.UnrestValue = 22
	s1.World.UnrestTier = unrestTierFromValue(s1.World.UnrestValue)
	s1.World.Situation = deriveSituation(s1.World.GrainTier, s1.World.UnrestTier)
	s1.Policies.TaxRatePct = 15
	s1.Policies.PermitRequiredHighRisk = true
	s1.Events = nil
	s1.Chat = nil
	s1.Messages = nil
	s1.NextEventID = 0
	s1.NextChatID = 0
	s1.NextMessageID = 0
	s1.TickCount = 42
	s1.NextContractID = 8
	s1.LastCleanupDate = "2026-02-12"

	p := &Player{ID: "p1", Name: "Ash Crow", Gold: 33, Rep: 7, Heat: 2, LastSeen: now, LocationID: locationCapital}
	s1.Players[p.ID] = p
	s1.Contracts["c1"] = &Contract{ID: "c1", Type: "Emergency", Status: "Issued", DeadlineTicks: 3, IssuedAtTick: s1.TickCount}
	s1.Events = append(s1.Events, Event{ID: 1, Type: "World", Severity: 1, Text: "Test event", At: now})
	s1.Chat = append(s1.Chat, ChatMessage{ID: 1, FromPlayerID: p.ID, FromName: p.Name, Text: "hello", At: now, Kind: "global"})
	s1.Messages = append(s1.Messages, DiplomaticMessage{ID: 1, FromPlayerID: p.ID, FromName: p.Name, ToPlayerID: p.ID, ToName: p.Name, Subject: "s", Body: "b", At: now})
	s1.LastActionAt[p.ID] = now

	if err := repo.Save(context.Background(), s1); err != nil {
		t.Fatalf("repo.Save error: %v", err)
	}

	s2 := newStore()
	if err := repo.LoadInto(context.Background(), s2); err != nil {
		t.Fatalf("repo.LoadInto error: %v", err)
	}

	if s2.World.DayNumber != s1.World.DayNumber || s2.World.GrainSupply != s1.World.GrainSupply {
		t.Fatalf("world mismatch after round-trip: got %+v want %+v", s2.World, s1.World)
	}
	if s2.Policies.TaxRatePct != 15 || !s2.Policies.PermitRequiredHighRisk {
		t.Fatalf("policy mismatch after round-trip: %+v", s2.Policies)
	}
	if s2.TickCount != 42 || s2.NextContractID != 8 {
		t.Fatalf("runtime counters mismatch: tick=%d nextContract=%d", s2.TickCount, s2.NextContractID)
	}
	if s2.LastCleanupDate != "2026-02-12" {
		t.Fatalf("last cleanup mismatch: %q", s2.LastCleanupDate)
	}
	if got := s2.Players[p.ID]; got == nil || got.Name != p.Name || got.Gold != p.Gold {
		t.Fatalf("player mismatch after round-trip: got=%+v", got)
	}
	if got := s2.Contracts["c1"]; got == nil || got.Type != "Emergency" || got.Status != "Issued" {
		t.Fatalf("contract mismatch after round-trip: got=%+v", got)
	}
	if len(s2.Events) != 1 || s2.Events[0].Text != "Test event" {
		t.Fatalf("events mismatch after round-trip: %+v", s2.Events)
	}
	if len(s2.Chat) != 1 || s2.Chat[0].Text != "hello" {
		t.Fatalf("chat mismatch after round-trip: %+v", s2.Chat)
	}
	if len(s2.Messages) != 1 || s2.Messages[0].Subject != "s" {
		t.Fatalf("messages mismatch after round-trip: %+v", s2.Messages)
	}
	if got, ok := s2.LastActionAt[p.ID]; !ok || !got.Equal(now) {
		t.Fatalf("runtime map LastActionAt mismatch: ok=%v got=%v want=%v", ok, got, now)
	}
	if s2.LastDeliverAt == nil || s2.DailyActionDate == nil || s2.DailyHighImpactN == nil {
		t.Fatalf("ensureRuntimeMaps should initialize runtime maps")
	}
}

func TestNewConfiguredStoreWithSQLite(t *testing.T) {
	t.Setenv("DB_DIALECT", "sqlite")
	t.Setenv("DB_SQLITE_PATH", filepath.Join(t.TempDir(), "config.sqlite"))

	store, err := newConfiguredStore()
	if err != nil {
		t.Fatalf("newConfiguredStore error: %v", err)
	}
	if store.repo == nil {
		t.Fatalf("expected configured store to include sqlite repo")
	}
	defer store.repo.db.Close()

	if store.Players == nil || store.Contracts == nil || store.Seats == nil {
		t.Fatalf("expected initialized store maps")
	}
}
