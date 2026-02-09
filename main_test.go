package main

import (
	"math/rand"
	"testing"
)

func TestComputeGrainTier(t *testing.T) {
	cases := []struct {
		supply int
		want   GrainTier
	}{
		{201, GrainStable},
		{200, GrainTight},
		{150, GrainTight},
		{101, GrainTight},
		{100, GrainScarce},
		{41, GrainScarce},
		{40, GrainCritical},
		{0, GrainCritical},
	}

	for _, tc := range cases {
		if got := computeGrainTier(tc.supply); got != tc.want {
			t.Fatalf("computeGrainTier(%d) = %s, want %s", tc.supply, got, tc.want)
		}
	}
}

func TestComputeUnrestTier(t *testing.T) {
	cases := []struct {
		value int
		want  UnrestTier
	}{
		{0, UnrestCalm},
		{10, UnrestCalm},
		{11, UnrestUneasy},
		{30, UnrestUneasy},
		{31, UnrestUnstable},
		{60, UnrestUnstable},
		{61, UnrestRioting},
	}

	for _, tc := range cases {
		if got := computeUnrestTier(tc.value); got != tc.want {
			t.Fatalf("computeUnrestTier(%d) = %s, want %s", tc.value, got, tc.want)
		}
	}
}

func TestAdvanceTime(t *testing.T) {
	world := newWorld(rand.New(rand.NewSource(1)))
	world.advanceTime()
	if world.Subphase != Evening || world.DayNumber != 1 {
		t.Fatalf("expected Day 1 Evening, got Day %d %s", world.DayNumber, world.Subphase)
	}
	world.advanceTime()
	if world.Subphase != Morning || world.DayNumber != 2 {
		t.Fatalf("expected Day 2 Morning, got Day %d %s", world.DayNumber, world.Subphase)
	}
}

func TestCriticalStreakPenalty(t *testing.T) {
	world := newWorld(rand.New(rand.NewSource(2)))
	world.GrainTier = GrainCritical
	for i := 0; i < 3; i++ {
		if world.updateCriticalStreak() {
			t.Fatalf("unexpected penalty before 4th critical tick")
		}
	}
	if !world.updateCriticalStreak() {
		t.Fatalf("expected penalty on 4th consecutive critical tick")
	}
	if !world.CriticalStreakPenaltyApplied {
		t.Fatalf("expected CriticalStreakPenaltyApplied to be true")
	}

	world.GrainTier = GrainScarce
	if world.updateCriticalStreak() {
		t.Fatalf("unexpected penalty after breaking streak")
	}
	if world.CriticalTickStreak != 0 || world.CriticalStreakPenaltyApplied {
		t.Fatalf("expected critical streak reset after breaking")
	}
}

func TestUpdateUnrestClamp(t *testing.T) {
	world := newWorld(rand.New(rand.NewSource(3)))
	world.UnrestValue = 95
	world.updateUnrest(3.0, true, 0, 2)
	if world.UnrestValue != 100 {
		t.Fatalf("expected unrest to clamp to 100, got %d", world.UnrestValue)
	}

	world.UnrestValue = 5
	world.updateUnrest(1.0, false, 2, 0)
	if world.UnrestValue != 0 {
		t.Fatalf("expected unrest to clamp to 0, got %d", world.UnrestValue)
	}
}

func TestContractFulfilledDeterministic(t *testing.T) {
	seed := int64(5)
	predictor := rand.New(rand.NewSource(seed))
	roll := predictor.Float64()

	world := newWorld(rand.New(rand.NewSource(seed)))
	got := world.contractFulfilled(GrainStable)
	want := roll < 0.70
	if got != want {
		t.Fatalf("contractFulfilled(Stable) = %t, want %t (roll %.6f)", got, want, roll)
	}
}
