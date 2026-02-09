package main

import (
	"bufio"
	"flag"
	"fmt"
	"math"
	"math/rand"
	"os"
	"strings"
	"time"
)

type Subphase string

type GrainTier string

type UnrestTier string

type ContractType string

type ContractStatus string

type EventType string

const (
	Morning Subphase = "Morning"
	Evening Subphase = "Evening"

	GrainStable   GrainTier = "Stable"
	GrainTight    GrainTier = "Tight"
	GrainScarce   GrainTier = "Scarce"
	GrainCritical GrainTier = "Critical"

	UnrestCalm     UnrestTier = "Calm"
	UnrestUneasy   UnrestTier = "Uneasy"
	UnrestUnstable UnrestTier = "Unstable"
	UnrestRioting  UnrestTier = "Rioting"

	ContractEmergency ContractType = "Emergency"
	ContractSmuggling ContractType = "Smuggling"

	ContractIssued    ContractStatus = "Issued"
	ContractFulfilled ContractStatus = "Fulfilled"
	ContractFailed    ContractStatus = "Failed"

	EventGrainTierChange   EventType = "GrainTierChange"
	EventUnrestTierChange  EventType = "UnrestTierChange"
	EventFactionAction     EventType = "FactionAction"
	EventContractIssued    EventType = "ContractIssued"
	EventContractFailed    EventType = "ContractFailed"
	EventMarketRestriction EventType = "MarketRestriction"
)

type Contract struct {
	ID            int
	Type          ContractType
	DeadlineTicks int
	Status        ContractStatus
}

type Event struct {
	DayNumber int
	Subphase  Subphase
	Type      EventType
	Severity  int
	Text      string
}

type World struct {
	DayNumber                    int
	Subphase                     Subphase
	GrainSupply                  int
	GrainTier                    GrainTier
	MarketPriceMultiplier        float64
	UnrestValue                  int
	UnrestTier                   UnrestTier
	RestrictedMarketsTicks       int
	Contracts                    []Contract
	CriticalTickStreak           int
	CriticalStreakPenaltyApplied bool
	nextContractID               int
	rng                          *rand.Rand
}

func main() {
	seedFlag := flag.Int64("seed", 0, "seed for rng")
	flag.Parse()

	seed := *seedFlag
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	rng := rand.New(rand.NewSource(seed))

	world := newWorld(rng)
	reader := bufio.NewReader(os.Stdin)

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		switch strings.ToLower(line) {
		case "advance":
			events := world.Advance()
			renderTick(world, events)
		case "status":
			renderStatus(world)
		case "quit":
			return
		default:
			fmt.Println("Unknown command. Available: advance, status, quit")
		}
	}
}

func newWorld(rng *rand.Rand) *World {
	grainSupply := 300
	grainTier := computeGrainTier(grainSupply)
	unrestValue := 0
	unrestTier := computeUnrestTier(unrestValue)
	marketMultiplier := marketMultiplier(grainTier)

	return &World{
		DayNumber:              1,
		Subphase:               Morning,
		GrainSupply:            grainSupply,
		GrainTier:              grainTier,
		MarketPriceMultiplier:  marketMultiplier,
		UnrestValue:            unrestValue,
		UnrestTier:             unrestTier,
		RestrictedMarketsTicks: 0,
		Contracts:              []Contract{},
		CriticalTickStreak:     0,
		rng:                    rng,
		nextContractID:         1,
	}
}

func (w *World) Advance() []Event {
	events := []Event{}
	prevGrainTier := w.GrainTier
	prevUnrestTier := w.UnrestTier

	w.advanceTime()

	w.consumeGrain()
	w.applyShocks()

	contextGrainTier := computeGrainTier(w.GrainSupply)
	contextUnrestTier := computeUnrestTier(w.UnrestValue)

	events = append(events, w.handleFactions(contextGrainTier, contextUnrestTier)...)

	fulfilledCount, failedCount, contractEvents := w.resolveContracts(contextGrainTier)
	events = append(events, contractEvents...)

	w.decayRestrictions()

	w.GrainTier = computeGrainTier(w.GrainSupply)
	w.MarketPriceMultiplier = marketMultiplier(w.GrainTier)
	effectiveMultiplier := w.MarketPriceMultiplier
	if w.RestrictedMarketsTicks > 0 {
		effectiveMultiplier = math.Max(1.0, w.MarketPriceMultiplier-0.5)
	}

	criticalPenalty := w.updateCriticalStreak()

	w.updateUnrest(effectiveMultiplier, criticalPenalty, fulfilledCount, failedCount)

	if w.GrainTier != prevGrainTier {
		events = append(events, Event{
			DayNumber: w.DayNumber,
			Subphase:  w.Subphase,
			Type:      EventGrainTierChange,
			Severity:  tierSeverity(w.GrainTier),
			Text:      grainTierNarrative(w.GrainTier, prevGrainTier),
		})
	}

	if w.UnrestTier != prevUnrestTier {
		events = append(events, Event{
			DayNumber: w.DayNumber,
			Subphase:  w.Subphase,
			Type:      EventUnrestTierChange,
			Severity:  unrestSeverity(w.UnrestTier),
			Text:      unrestTierNarrative(w.UnrestTier, prevUnrestTier),
		})
	}

	return events
}

func (w *World) advanceTime() {
	if w.Subphase == Morning {
		w.Subphase = Evening
		return
	}
	w.Subphase = Morning
	w.DayNumber++
}

func (w *World) consumeGrain() {
	w.GrainSupply -= 18 + w.rng.Intn(9)
	if w.GrainSupply < 0 {
		w.GrainSupply = 0
	}
}

func (w *World) applyShocks() {
	if w.rng.Float64() < 0.10 {
		w.GrainSupply -= 25
		if w.GrainSupply < 0 {
			w.GrainSupply = 0
		}
	}

	if w.rng.Float64() < 0.08 {
		currentTier := computeGrainTier(w.GrainSupply)
		switch currentTier {
		case GrainTight, GrainScarce, GrainCritical:
			w.GrainSupply += 20
		}
	}
}

func (w *World) handleFactions(grainTier GrainTier, unrestTier UnrestTier) []Event {
	events := []Event{}

	if (unrestTier == UnrestUnstable || unrestTier == UnrestRioting || grainTier == GrainCritical) && !w.hasActiveContract(ContractEmergency) {
		w.issueContract(ContractEmergency, 4)
		events = append(events, Event{
			DayNumber: w.DayNumber,
			Subphase:  w.Subphase,
			Type:      EventFactionAction,
			Severity:  4,
			Text:      "[City Authority] requisitions emergency shipments.",
		})
		events = append(events, Event{
			DayNumber: w.DayNumber,
			Subphase:  w.Subphase,
			Type:      EventContractIssued,
			Severity:  3,
			Text:      "An emergency delivery contract is announced.",
		})
	}

	if unrestTier == UnrestRioting && w.RestrictedMarketsTicks == 0 {
		w.RestrictedMarketsTicks = 2
		events = append(events, Event{
			DayNumber: w.DayNumber,
			Subphase:  w.Subphase,
			Type:      EventMarketRestriction,
			Severity:  4,
			Text:      "[City Authority] imposes strict market controls.",
		})
	}

	if (grainTier == GrainScarce || grainTier == GrainCritical) && !w.hasActiveContract(ContractSmuggling) {
		w.issueContract(ContractSmuggling, 3)
		events = append(events, Event{
			DayNumber: w.DayNumber,
			Subphase:  w.Subphase,
			Type:      EventFactionAction,
			Severity:  3,
			Text:      "[Merchant League] issues smuggling orders.",
		})
		events = append(events, Event{
			DayNumber: w.DayNumber,
			Subphase:  w.Subphase,
			Type:      EventContractIssued,
			Severity:  2,
			Text:      "A discreet smuggling contract circulates in back rooms.",
		})
	}

	return events
}

func (w *World) resolveContracts(grainTier GrainTier) (int, int, []Event) {
	fulfilled := 0
	failed := 0
	events := []Event{}

	for i := range w.Contracts {
		contract := &w.Contracts[i]
		if contract.Status != ContractIssued {
			continue
		}

		if w.contractFulfilled(grainTier) {
			contract.Status = ContractFulfilled
			fulfilled++
			switch contract.Type {
			case ContractEmergency:
				w.GrainSupply += 60
			case ContractSmuggling:
				w.GrainSupply += 30
			}
			continue
		}

		contract.DeadlineTicks--
		if contract.DeadlineTicks <= 0 {
			contract.Status = ContractFailed
			failed++
			events = append(events, Event{
				DayNumber: w.DayNumber,
				Subphase:  w.Subphase,
				Type:      EventContractFailed,
				Severity:  4,
				Text:      "A contract has failed, raising tension in the city.",
			})
		}
	}

	return fulfilled, failed, events
}

func (w *World) decayRestrictions() {
	if w.RestrictedMarketsTicks > 0 {
		w.RestrictedMarketsTicks--
	}
}

func (w *World) updateCriticalStreak() bool {
	if w.GrainTier != GrainCritical {
		w.CriticalTickStreak = 0
		w.CriticalStreakPenaltyApplied = false
		return false
	}

	w.CriticalTickStreak++
	if w.CriticalTickStreak >= 4 && !w.CriticalStreakPenaltyApplied {
		w.CriticalStreakPenaltyApplied = true
		return true
	}

	return false
}

func (w *World) updateUnrest(multiplier float64, criticalPenalty bool, fulfilledCount int, failedCount int) {
	if multiplier >= 2.0 {
		w.UnrestValue += 5
	}
	if criticalPenalty {
		w.UnrestValue += 10
	}
	w.UnrestValue += failedCount * 15
	w.UnrestValue -= fulfilledCount * 10

	if w.UnrestValue < 0 {
		w.UnrestValue = 0
	}
	if w.UnrestValue > 100 {
		w.UnrestValue = 100
	}

	w.UnrestTier = computeUnrestTier(w.UnrestValue)
}

func (w *World) hasActiveContract(contractType ContractType) bool {
	for _, contract := range w.Contracts {
		if contract.Type == contractType && contract.Status == ContractIssued {
			return true
		}
	}
	return false
}

func (w *World) issueContract(contractType ContractType, deadline int) {
	w.Contracts = append(w.Contracts, Contract{
		ID:            w.nextContractID,
		Type:          contractType,
		DeadlineTicks: deadline,
		Status:        ContractIssued,
	})
	w.nextContractID++
}

func (w *World) contractFulfilled(grainTier GrainTier) bool {
	roll := w.rng.Float64()
	switch grainTier {
	case GrainStable:
		return roll < 0.70
	case GrainTight:
		return roll < 0.55
	case GrainScarce:
		return roll < 0.40
	case GrainCritical:
		return roll < 0.25
	default:
		return false
	}
}

func computeGrainTier(grainSupply int) GrainTier {
	switch {
	case grainSupply > 200:
		return GrainStable
	case grainSupply > 100:
		return GrainTight
	case grainSupply > 40:
		return GrainScarce
	default:
		return GrainCritical
	}
}

func computeUnrestTier(unrestValue int) UnrestTier {
	switch {
	case unrestValue <= 10:
		return UnrestCalm
	case unrestValue <= 30:
		return UnrestUneasy
	case unrestValue <= 60:
		return UnrestUnstable
	default:
		return UnrestRioting
	}
}

func marketMultiplier(grainTier GrainTier) float64 {
	switch grainTier {
	case GrainStable:
		return 1.0
	case GrainTight:
		return 1.5
	case GrainScarce:
		return 2.0
	case GrainCritical:
		return 3.0
	default:
		return 1.0
	}
}

func grainTierNarrative(tier GrainTier, prev GrainTier) string {
	if tier == GrainStable || tier == GrainTight || tier == GrainScarce {
		if prev == GrainCritical || prev == GrainScarce && tier != GrainScarce {
			return "Fresh grain reaches the markets, easing shortages."
		}
	}
	switch tier {
	case GrainTight:
		return "Grain supply tightens."
	case GrainScarce:
		return "Grain stores thin across the city."
	case GrainCritical:
		return "Grain stores fall below emergency reserves."
	case GrainStable:
		return "Fresh grain reaches the markets, easing shortages."
	default:
		return "Grain conditions shift across the city."
	}
}

func unrestTierNarrative(tier UnrestTier, prev UnrestTier) string {
	if tier == UnrestCalm && prev != UnrestCalm {
		return "The streets quiet as tensions ease."
	}
	switch tier {
	case UnrestUneasy:
		return "Whispers of worry spread through the streets."
	case UnrestUnstable:
		return "Tension rises as crowds gather and tempers flare."
	case UnrestRioting:
		return "The city erupts into open unrest."
	case UnrestCalm:
		return "The streets quiet as tensions ease."
	default:
		return "The city's mood shifts in uncertain ways."
	}
}

func tierSeverity(tier GrainTier) int {
	switch tier {
	case GrainStable:
		return 1
	case GrainTight:
		return 2
	case GrainScarce:
		return 3
	case GrainCritical:
		return 5
	default:
		return 1
	}
}

func unrestSeverity(tier UnrestTier) int {
	switch tier {
	case UnrestCalm:
		return 1
	case UnrestUneasy:
		return 2
	case UnrestUnstable:
		return 3
	case UnrestRioting:
		return 5
	default:
		return 1
	}
}

func renderTick(world *World, events []Event) {
	fmt.Printf("Day %d – %s\n", world.DayNumber, world.Subphase)
	if len(events) == 0 {
		fmt.Println("(no events today)")
		return
	}
	for _, event := range events {
		fmt.Println(event.Text)
	}
}

func renderStatus(world *World) {
	fmt.Printf("Day %d – %s\n", world.DayNumber, world.Subphase)
	fmt.Printf("Grain: %d (%s)\n", world.GrainSupply, world.GrainTier)
	fmt.Printf("Unrest: %d (%s)\n", world.UnrestValue, world.UnrestTier)
	fmt.Printf("Restricted markets ticks: %d\n", world.RestrictedMarketsTicks)
	if len(world.Contracts) == 0 {
		fmt.Println("Contracts: none")
		return
	}
	fmt.Println("Contracts:")
	for _, contract := range world.Contracts {
		fmt.Printf("- #%d %s (%s), deadline: %d\n", contract.ID, contract.Type, contract.Status, contract.DeadlineTicks)
	}
}
