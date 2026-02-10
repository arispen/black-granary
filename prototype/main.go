package main

import (
	"fmt"
	"html/template"
	"log"
	"math/rand"
	"net/http"
	"path/filepath"
	"sync"
	"time"
)

type WorldState struct {
	DayNumber                    int
	Subphase                     string
	GrainSupply                  int
	GrainTier                    string
	UnrestValue                  int
	UnrestTier                   string
	RestrictedMarketsTicks       int
	CriticalTickStreak           int
	CriticalStreakPenaltyApplied bool
}

type Contract struct {
	ID            string
	Type          string
	DeadlineTicks int
	Status        string
}

type Event struct {
	ID        int64
	DayNumber int
	Subphase  string
	Type      string
	Severity  int
	Text      string
}

type Store struct {
	mu             sync.Mutex
	world          WorldState
	contracts      map[string]*Contract
	events         []Event
	nextEventID    int64
	nextContractID int64
	rng            *rand.Rand
}

type ViewData struct {
	World     WorldState
	Contracts []*Contract
	Events    []Event
}

const maxEvents = 200

func main() {
	templates := parseTemplates()
	store := newStore()

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		store.mu.Lock()
		defer store.mu.Unlock()
		data := store.viewData()
		renderFullPage(w, templates, data)
	})
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		store.mu.Lock()
		defer store.mu.Unlock()
		data := store.viewData()
		renderFragments(w, templates, data)
	})
	mux.HandleFunc("/action", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		store.mu.Lock()
		defer store.mu.Unlock()

		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		action := r.FormValue("action")
		contractID := r.FormValue("contract_id")

		fulfilledFromAction := 0
		switch action {
		case "advance":
			store.addEvent("Time passes under mounting pressure.", "Time", 2)
		case "accept":
			if contract := store.contracts[contractID]; contract != nil && contract.Status == "Issued" {
				contract.Status = "Accepted"
				store.addEvent("[You] accept a risky contract under tightening conditions.", "Player", 2)
			}
		case "ignore":
			if contract := store.contracts[contractID]; contract != nil && contract.Status == "Issued" {
				contract.Status = "Ignored"
				store.addEvent("[You] turn away from the contract, letting others decide its fate.", "Player", 2)
			}
		case "investigate":
			store.world.UnrestValue = clamp(store.world.UnrestValue-5, 0, 100)
			store.addEvent("[You] investigate rumors around the supply routes.", "Player", 2)
		case "deliver":
			if contract := store.contracts[contractID]; contract != nil && contract.Status == "Accepted" {
				chance := deliverChance(store.world.GrainTier)
				if rollPercent(store.rng) < chance {
					contract.Status = "Fulfilled"
					fulfilledFromAction++
					applyContractReward(&store.world, contract)
					store.addEvent("[You] deliver supplies through tense streets.", "Player", 3)
				} else {
					store.addEvent("[You] attempt a delivery, but it collapses at the last moment.", "Player", 3)
				}
			}
		}

		tick(store, fulfilledFromAction)
		data := store.viewData()
		renderFragments(w, templates, data)
	})

	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}
	log.Println("listening on http://localhost:8080")
	log.Fatal(server.ListenAndServe())
}

func newStore() *Store {
	store := &Store{
		world: WorldState{
			DayNumber:              1,
			Subphase:               "Morning",
			GrainSupply:            300,
			GrainTier:              "Stable",
			UnrestValue:            5,
			UnrestTier:             "Calm",
			RestrictedMarketsTicks: 0,
		},
		contracts: make(map[string]*Contract),
		events:    make([]Event, 0, maxEvents),
		rng:       rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	store.addEvent("The city stirs as merchants assess the granaries.", "Opening", 2)
	return store
}

func parseTemplates() *template.Template {
	base := template.Must(template.ParseGlob(filepath.Join("templates", "*.html")))
	return template.Must(base.ParseGlob(filepath.Join("templates", "fragments", "*.html")))
}

func renderFullPage(w http.ResponseWriter, templates *template.Template, data ViewData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.ExecuteTemplate(w, "base", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func renderFragments(w http.ResponseWriter, templates *template.Template, data ViewData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// The dashboard fragment replaces #dashboard, while the event log uses hx-swap-oob to
	// refresh #event-log without a full-page reload.
	if err := templates.ExecuteTemplate(w, "dashboard", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := templates.ExecuteTemplate(w, "events", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Store) viewData() ViewData {
	contracts := make([]*Contract, 0, len(s.contracts))
	for _, contract := range s.contracts {
		contracts = append(contracts, contract)
	}
	return ViewData{
		World:     s.world,
		Contracts: contracts,
		Events:    append([]Event(nil), s.events...),
	}
}

func (s *Store) addEvent(text, eventType string, severity int) {
	s.nextEventID++
	s.events = append(s.events, Event{
		ID:        s.nextEventID,
		DayNumber: s.world.DayNumber,
		Subphase:  s.world.Subphase,
		Type:      eventType,
		Severity:  severity,
		Text:      text,
	})
	if len(s.events) > maxEvents {
		s.events = s.events[len(s.events)-maxEvents:]
	}
}

func tick(store *Store, fulfilledFromAction int) {
	world := &store.world
	prevGrainTier := world.GrainTier
	prevUnrestTier := world.UnrestTier

	world.GrainSupply -= 18 + store.rng.Intn(9)
	world.GrainSupply = clamp(world.GrainSupply, 0, 10000)

	if store.rng.Float64() < 0.10 {
		world.GrainSupply = clamp(world.GrainSupply-25, 0, 10000)
	}
	midTier := grainTierFromSupply(world.GrainSupply)
	if store.rng.Float64() < 0.08 && midTier != "Stable" {
		world.GrainSupply += 20
	}

	currentGrainTier := grainTierFromSupply(world.GrainSupply)
	currentUnrestTier := unrestTierFromValue(world.UnrestValue)

	if currentUnrestTier == "Unstable" || currentUnrestTier == "Rioting" || currentGrainTier == "Critical" {
		if !store.hasActiveContract("Emergency") {
			store.issueContract("Emergency", 4)
			store.addEvent("[City Authority] requisitions emergency shipments.", "Faction", 3)
			store.addEvent("A new contract is posted amid rising pressure.", "Contract", 2)
		}
		if currentUnrestTier == "Rioting" && world.RestrictedMarketsTicks == 0 {
			world.RestrictedMarketsTicks = 2
			store.addEvent("[City Authority] imposes strict market controls.", "Faction", 4)
		}
	}

	if currentGrainTier == "Scarce" || currentGrainTier == "Critical" {
		if !store.hasActiveContract("Smuggling") {
			store.issueContract("Smuggling", 3)
			store.addEvent("[Merchant League] issues smuggling orders.", "Faction", 3)
			store.addEvent("A new contract is posted amid rising pressure.", "Contract", 2)
		}
	}

	fulfilledThisTick := fulfilledFromAction
	failedThisTick := 0

	for _, contract := range store.contracts {
		if contract.Status != "Issued" && contract.Status != "Accepted" && contract.Status != "Ignored" {
			continue
		}
		chance := autoFulfillChance(currentGrainTier)
		if contract.Status == "Accepted" {
			chance += 15
			if chance > 95 {
				chance = 95
			}
		}
		if rollPercent(store.rng) < chance {
			contract.Status = "Fulfilled"
			fulfilledThisTick++
			applyContractReward(world, contract)
			continue
		}

		contract.DeadlineTicks--
		if contract.DeadlineTicks <= 0 {
			contract.Status = "Failed"
			failedThisTick++
			store.addEvent("A contract has failed, raising tension in the city.", "Contract", 4)
		}
	}

	if world.RestrictedMarketsTicks > 0 {
		world.RestrictedMarketsTicks--
	}

	world.GrainTier = grainTierFromSupply(world.GrainSupply)

	criticalPenaltyTriggered := false
	if world.GrainTier == "Critical" {
		world.CriticalTickStreak++
		if world.CriticalTickStreak >= 4 && !world.CriticalStreakPenaltyApplied {
			world.CriticalStreakPenaltyApplied = true
			criticalPenaltyTriggered = true
		}
	} else {
		world.CriticalTickStreak = 0
		world.CriticalStreakPenaltyApplied = false
	}

	multiplier := effectiveMultiplier(world.GrainTier, world.RestrictedMarketsTicks)
	if multiplier >= 2.0 {
		world.UnrestValue += 5
	}
	if criticalPenaltyTriggered {
		world.UnrestValue += 10
	}
	world.UnrestValue += failedThisTick * 15
	world.UnrestValue -= fulfilledThisTick * 10
	world.UnrestValue = clamp(world.UnrestValue, 0, 100)
	world.UnrestTier = unrestTierFromValue(world.UnrestValue)

	if world.GrainTier != prevGrainTier {
		store.addEvent(grainTierNarrative(prevGrainTier, world.GrainTier), "World", 3)
	}
	if world.UnrestTier != prevUnrestTier {
		store.addEvent(unrestTierNarrative(prevUnrestTier, world.UnrestTier), "World", 3)
	}

	advanceTime(world)
}

func advanceTime(world *WorldState) {
	if world.Subphase == "Morning" {
		world.Subphase = "Evening"
		return
	}
	world.Subphase = "Morning"
	world.DayNumber++
}

func (s *Store) issueContract(contractType string, deadline int) {
	s.nextContractID++
	id := fmt.Sprintf("C%03d", s.nextContractID)
	s.contracts[id] = &Contract{
		ID:            id,
		Type:          contractType,
		DeadlineTicks: deadline,
		Status:        "Issued",
	}
}

func (s *Store) hasActiveContract(contractType string) bool {
	for _, contract := range s.contracts {
		if contract.Type == contractType && (contract.Status == "Issued" || contract.Status == "Accepted" || contract.Status == "Ignored") {
			return true
		}
	}
	return false
}

func grainTierFromSupply(supply int) string {
	switch {
	case supply > 200:
		return "Stable"
	case supply >= 101:
		return "Tight"
	case supply >= 41:
		return "Scarce"
	default:
		return "Critical"
	}
}

func unrestTierFromValue(value int) string {
	switch {
	case value <= 10:
		return "Calm"
	case value <= 30:
		return "Uneasy"
	case value <= 60:
		return "Unstable"
	default:
		return "Rioting"
	}
}

func effectiveMultiplier(grainTier string, restrictedTicks int) float64 {
	base := map[string]float64{
		"Stable":   1.0,
		"Tight":    1.5,
		"Scarce":   2.0,
		"Critical": 3.0,
	}[grainTier]
	if restrictedTicks > 0 {
		base -= 0.5
		if base < 1.0 {
			base = 1.0
		}
	}
	return base
}

func autoFulfillChance(grainTier string) int {
	switch grainTier {
	case "Stable":
		return 70
	case "Tight":
		return 55
	case "Scarce":
		return 40
	default:
		return 25
	}
}

func deliverChance(grainTier string) int {
	switch grainTier {
	case "Stable":
		return 80
	case "Tight":
		return 65
	case "Scarce":
		return 50
	default:
		return 35
	}
}

func applyContractReward(world *WorldState, contract *Contract) {
	switch contract.Type {
	case "Emergency":
		world.GrainSupply += 60
	case "Smuggling":
		world.GrainSupply += 30
	}
}

func grainTierNarrative(prev, current string) string {
	if tierRank(current) > tierRank(prev) {
		return "Fresh grain reaches the markets, easing shortages."
	}
	switch current {
	case "Tight":
		return "Grain supply tightens."
	case "Scarce":
		return "Grain stores thin across the city."
	case "Critical":
		return "Grain stores fall below emergency reserves."
	default:
		return "Fresh grain reaches the markets, easing shortages."
	}
}

func unrestTierNarrative(prev, current string) string {
	if unrestRank(current) < unrestRank(prev) {
		return "The streets quiet as tensions ease."
	}
	switch current {
	case "Uneasy":
		return "Whispers of worry spread through the streets."
	case "Unstable":
		return "Tension rises as crowds gather and tempers flare."
	case "Rioting":
		return "The city erupts into open unrest."
	default:
		return "The streets quiet as tensions ease."
	}
}

func tierRank(tier string) int {
	order := map[string]int{"Stable": 0, "Tight": 1, "Scarce": 2, "Critical": 3}
	return order[tier]
}

func unrestRank(tier string) int {
	order := map[string]int{"Calm": 0, "Uneasy": 1, "Unstable": 2, "Rioting": 3}
	return order[tier]
}

func rollPercent(rng *rand.Rand) int {
	return rng.Intn(100) + 1
}

func clamp(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
