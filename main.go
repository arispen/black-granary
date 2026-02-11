package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"html/template"
	"io"
	"log"
	mathrand "math/rand"
	"net"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	cookieName            = "pid"
	maxEvents             = 300
	maxChat               = 200
	maxVisibleContracts   = 12
	onlineWindow          = 60 * time.Second
	inactiveWindow        = 120 * time.Second
	actionCooldown        = 2 * time.Second
	deliverCooldown       = 10 * time.Second
	chatCooldown          = 2 * time.Second
	adminToken            = "DEV"
	serverAddr            = ":8080"
	templateRoot          = "templates"
	initialPlayerGold     = 20
	rumorInvestigateGain  = 1
	rumorWhisperGain      = 1
	rumorDeliverBonusGold = 3
	seatTenureTicks       = 8
	electionWindowTicks   = 2
	highImpactDailyCap    = 3
	loanDueTicks          = 4
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
	Situation                    string
}

type Player struct {
	ID                      string
	Name                    string
	Gold                    int
	Rep                     int
	Heat                    int
	Rumors                  int
	CompletedContracts      int
	CompletedContractsToday int
	CompletedTodayDateUTC   string
	RiteImmunityTicks       int
	LastSeen                time.Time
}

type Contract struct {
	ID            string
	Type          string
	DeadlineTicks int
	Status        string
	OwnerPlayerID string
	OwnerName     string
	Stance        string
	IssuedAtTick  int64
}

type Event struct {
	ID        int64
	DayNumber int
	Subphase  string
	Type      string
	Severity  int
	Text      string
	At        time.Time
}

type ChatMessage struct {
	ID           int64
	FromPlayerID string
	FromName     string
	ToPlayerID   string
	ToName       string
	Text         string
	At           time.Time
	Kind         string
}

type Institution struct {
	ID   string
	Name string
}

type Seat struct {
	ID                  string
	Name                string
	InstitutionID       string
	HolderPlayerID      string
	HolderName          string
	TenureTicksLeft     int
	ElectionWindowTicks int
}

type PolicyState struct {
	TaxRatePct             int
	PermitRequiredHighRisk bool
	SmugglingEmbargoTicks  int
}

type Rumor struct {
	ID             int64
	Claim          string
	Topic          string
	TargetPlayerID string
	TargetName     string
	SourcePlayerID string
	SourceName     string
	Credibility    int
	Spread         int
	Decay          int
}

type Evidence struct {
	ID             int64
	Topic          string
	TargetPlayerID string
	TargetName     string
	SourcePlayerID string
	SourceName     string
	Strength       int
	ExpiryTick     int64
}

type Loan struct {
	ID               string
	LenderPlayerID   string
	LenderName       string
	BorrowerPlayerID string
	BorrowerName     string
	Principal        int
	Remaining        int
	DueTick          int64
	Status           string
}

type Obligation struct {
	ID               string
	CreditorPlayerID string
	CreditorName     string
	DebtorPlayerID   string
	DebtorName       string
	Reason           string
	Severity         int
	DueTick          int64
	Status           string
}

type Store struct {
	mu sync.Mutex

	World        WorldState
	Players      map[string]*Player
	Contracts    map[string]*Contract
	Institutions map[string]*Institution
	Seats        map[string]*Seat
	Policies     PolicyState
	Rumors       map[int64]*Rumor
	Evidence     map[int64]*Evidence
	Loans        map[string]*Loan
	Obligations  map[string]*Obligation

	Events []Event
	Chat   []ChatMessage

	NextEventID      int64
	NextContractID   int64
	NextChatID       int64
	NextRumorID      int64
	NextEvidenceID   int64
	NextLoanID       int64
	NextObligationID int64

	LastDailyTickDate string
	LastTickAt        time.Time
	TickEvery         time.Duration
	TickCount         int64

	LastChatAt        map[string]time.Time
	LastActionAt      map[string]time.Time
	LastDeliverAt     map[string]time.Time
	LastInvestigateAt map[string]int64
	LastSeatActionAt  map[string]int64
	LastIntelActionAt map[string]int64
	DailyActionDate   map[string]string
	DailyHighImpactN  map[string]int
	ToastByPlayer     map[string]string

	rng *mathrand.Rand
}

type PlayerSummary struct {
	Name   string
	Rep    int
	Title  string
	Gold   int
	Online bool
}

type ContractView struct {
	ID            string
	Type          string
	Status        string
	DeadlineTicks int
	OwnerName     string
	Stance        string
	UrgencyClass  string
	CanAccept     bool
	CanIgnore     bool
	CanAbandon    bool
	CanDeliver    bool
	DeliverLabel  string
	DeliverDisabled bool
	ShowOutcome   bool
	OutcomeLabel  string
	OutcomeNote   string
}

type StandingView struct {
	ReputationValue int
	ReputationLabel string
	HeatValue       int
	HeatLabel       string
	WealthGold      int
	CompletedToday  int
	CompletedTotal  int
	Rumors          int
}

type EventView struct {
	DayNumber int
	Subphase  string
	Text      string
	At        string
}

type ChatView struct {
	FromName  string
	FromTitle string
	ToName    string
	Text      string
	Kind      string
	At        string
}

type SeatView struct {
	ID                  string
	Name                string
	InstitutionName     string
	HolderName          string
	TenureTicksLeft     int
	ElectionWindowTicks int
	IsElectionOpen      bool
	CanCampaign         bool
	CanChallenge        bool
	CanToggleTaxHigh    bool
	CanToggleTaxLow     bool
	CanTogglePermit     bool
	CanToggleEmbargo    bool
}

type RumorView struct {
	ID          int64
	Claim       string
	Topic       string
	TargetName  string
	SourceName  string
	Credibility int
	Spread      int
	Decay       int
}

type EvidenceView struct {
	ID         int64
	Topic      string
	TargetName string
	SourceName string
	Strength   int
	ExpiryIn   int64
}

type LoanView struct {
	ID           string
	LenderName   string
	BorrowerName string
	Remaining    int
	DueIn        int64
	Status       string
}

type ObligationView struct {
	ID           string
	CreditorName string
	DebtorName   string
	Reason       string
	Severity     int
	DueIn        int64
	Status       string
}

type PlayerOption struct {
	ID   string
	Name string
}

type PageData struct {
	NowUTC           string
	Player           *Player
	PlayerTitle      string
	Standing         StandingView
	World            WorldState
	Situation        string
	HighImpactRemaining int
	HighImpactCap       int
	InvestigateDisabled bool
	InvestigateLabel    string
	Contracts        []ContractView
	Events           []EventView
	Players          []PlayerSummary
	Chat             []ChatView
	ChatDraft        string
	Toast            string
	AcceptedCount    int
	VisibleContractN int
	TotalContractN   int
	Seats            []SeatView
	Policies         PolicyState
	Rumors           []RumorView
	Evidence         []EvidenceView
	Loans            []LoanView
	Obligations      []ObligationView
	PlayerOptions    []PlayerOption
	TickStatus       string
}

const (
	contractStanceCareful = "Careful"
	contractStanceFast    = "Fast"
	contractStanceQuiet   = "Quiet"
)

type DeliverOutcome struct {
	RewardGold int
	HeatDelta  int
	RepDelta   int
	Stance     string
}

var nameFirst = []string{"Ash", "Bran", "Corin", "Dain", "Elow", "Fenn", "Garr", "Hale", "Ira", "Jory", "Kael", "Liora", "Mara", "Nell", "Orin", "Perrin", "Quill", "Rysa", "Sable", "Tarin"}
var nameLast = []string{"Stone", "Vale", "Thorne", "Mire", "Brindle", "Hollow", "Reed", "Kestrel", "Cinder", "Rook", "Fen", "Crow", "Wick", "Hearth", "Barrow"}

func main() {
	tmpl := parseTemplates()
	store := newStore()
	startTickScheduler(store)
	mux := newMux(store, tmpl)

	log.Printf("listening on http://localhost%s", serverAddr)
	log.Fatal(http.ListenAndServe(serverAddr, mux))
}

func newMux(store *Store, tmpl *template.Template) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Concurrency model: lock for full handler to keep all reads/writes consistent and race-free.
		store.mu.Lock()
		defer store.mu.Unlock()

		p := ensurePlayerLocked(store, w, r)
		now := time.Now().UTC()
		p.LastSeen = now

		// Daily bootstrap tick: first login in a UTC date runs exactly one tick.
		today := now.Format("2006-01-02")
		if store.LastDailyTickDate != today {
			runWorldTickLocked(store, now)
			store.LastTickAt = now
			store.LastDailyTickDate = today
			addEventLocked(store, Event{Type: "Daily", Severity: 1, Text: "A new day dawns with fresh uncertainty.", At: now})
			setToastLocked(store, p.ID, "The city shifts with a new dawn.")
		}

		renderPage(w, tmpl, "base", buildPageDataLocked(store, p.ID, true))
	})

	mux.HandleFunc("/frag/dashboard", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		store.mu.Lock()
		defer store.mu.Unlock()
		p := ensurePlayerLocked(store, w, r)
		p.LastSeen = time.Now().UTC()
		renderActionLikeResponse(w, tmpl, buildPageDataLocked(store, p.ID, false), false)
	})

	mux.HandleFunc("/frag/events", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		store.mu.Lock()
		defer store.mu.Unlock()
		p := ensurePlayerLocked(store, w, r)
		p.LastSeen = time.Now().UTC()
		renderPage(w, tmpl, "events_inner", buildPageDataLocked(store, p.ID, false))
	})

	mux.HandleFunc("/frag/chat", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		store.mu.Lock()
		defer store.mu.Unlock()
		p := ensurePlayerLocked(store, w, r)
		p.LastSeen = time.Now().UTC()
		renderPage(w, tmpl, "chat_inner", buildPageDataLocked(store, p.ID, false))
	})

	mux.HandleFunc("/frag/players", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		store.mu.Lock()
		defer store.mu.Unlock()
		p := ensurePlayerLocked(store, w, r)
		p.LastSeen = time.Now().UTC()
		renderPage(w, tmpl, "players_inner", buildPageDataLocked(store, p.ID, false))
	})

	mux.HandleFunc("/frag/institutions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		store.mu.Lock()
		defer store.mu.Unlock()
		p := ensurePlayerLocked(store, w, r)
		p.LastSeen = time.Now().UTC()
		renderPage(w, tmpl, "institutions_inner", buildPageDataLocked(store, p.ID, false))
	})

	mux.HandleFunc("/frag/intel", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		store.mu.Lock()
		defer store.mu.Unlock()
		p := ensurePlayerLocked(store, w, r)
		p.LastSeen = time.Now().UTC()
		renderPage(w, tmpl, "intel_inner", buildPageDataLocked(store, p.ID, false))
	})

	mux.HandleFunc("/frag/ledger", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		store.mu.Lock()
		defer store.mu.Unlock()
		p := ensurePlayerLocked(store, w, r)
		p.LastSeen = time.Now().UTC()
		renderPage(w, tmpl, "ledger_inner", buildPageDataLocked(store, p.ID, false))
	})

	mux.HandleFunc("/action", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		store.mu.Lock()
		defer store.mu.Unlock()

		p := ensurePlayerLocked(store, w, r)
		now := time.Now().UTC()
		p.LastSeen = now

		if tooSoon(store.LastActionAt[p.ID], now, actionCooldown) {
			setToastLocked(store, p.ID, "Slow down.")
			renderActionLikeResponse(w, tmpl, buildPageDataLocked(store, p.ID, true), false)
			return
		}
		store.LastActionAt[p.ID] = now

		input := ActionInput{
			Action:     strings.TrimSpace(r.FormValue("action")),
			ContractID: strings.TrimSpace(r.FormValue("contract_id")),
			Stance:     strings.TrimSpace(r.FormValue("stance")),
			TargetID:   strings.TrimSpace(r.FormValue("target_id")),
			Claim:      strings.TrimSpace(r.FormValue("claim")),
			Topic:      strings.TrimSpace(r.FormValue("topic")),
			LoanID:     strings.TrimSpace(r.FormValue("loan_id")),
		}
		if n, err := strconv.Atoi(strings.TrimSpace(r.FormValue("amount"))); err == nil {
			input.Amount = n
		}

		handleActionInputLocked(store, p, now, input)
		renderActionLikeResponse(w, tmpl, buildPageDataLocked(store, p.ID, true), false)
	})

	mux.HandleFunc("/chat", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		store.mu.Lock()
		defer store.mu.Unlock()

		p := ensurePlayerLocked(store, w, r)
		now := time.Now().UTC()
		p.LastSeen = now
		rawMsg := r.FormValue("text")
		msg := strings.TrimSpace(rawMsg)

		if tooSoon(store.LastChatAt[p.ID], now, chatCooldown) {
			setToastLocked(store, p.ID, "Chat cooldown active.")
			data := buildPageDataLocked(store, p.ID, true)
			data.ChatDraft = rawMsg
			renderChatResponse(w, tmpl, data, true)
			return
		}

		if msg == "" {
			renderChatResponse(w, tmpl, buildPageDataLocked(store, p.ID, true), true)
			return
		}

		store.LastChatAt[p.ID] = now
		accepted := handleChatLocked(store, p, now, msg)
		data := buildPageDataLocked(store, p.ID, true)
		if !accepted {
			data.ChatDraft = rawMsg
		}
		renderChatResponse(w, tmpl, data, true)
	})

	mux.HandleFunc("/admin", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !isAdmin(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		store.mu.Lock()
		defer store.mu.Unlock()

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		tokenQ := ""
		if r.URL.Query().Get("token") == adminToken {
			tokenQ = "?token=" + adminToken
		}
		_, _ = fmt.Fprintf(w, "<!DOCTYPE html><html><head><meta charset=\"utf-8\"><title>Admin</title><style>body{font-family:ui-sans-serif,system-ui;background:#0b0f14;color:#e5ecf4;padding:24px}pre{background:#121923;border:1px solid #2a3442;padding:12px;border-radius:8px;overflow:auto}button{background:#1f6feb;color:#fff;border:0;padding:8px 12px;border-radius:6px;margin-right:8px;cursor:pointer}</style></head><body>")
		_, _ = fmt.Fprintf(w, "<h1>Admin</h1><p>RESET ON RESTART (test realm)</p>")
		_, _ = fmt.Fprintf(w, "<form style=\"display:inline\" method=\"post\" action=\"/admin/tick%s\"><button type=\"submit\">Force Tick</button></form>", tokenQ)
		_, _ = fmt.Fprintf(w, "<form style=\"display:inline\" method=\"post\" action=\"/admin/reset%s\"><button type=\"submit\">Reset World</button></form>", tokenQ)

		online := onlinePlayersLocked(store, time.Now().UTC())
		_, _ = fmt.Fprintf(w, "<h2>World</h2><pre>%+v</pre>", store.World)
		_, _ = fmt.Fprintf(w, "<h2>Active Contracts</h2><pre>")
		for _, c := range sortedContractsLocked(store) {
			if c.Status == "Issued" || c.Status == "Accepted" {
				_, _ = fmt.Fprintf(w, "%+v\n", *c)
			}
		}
		_, _ = fmt.Fprintf(w, "</pre><h2>Online Players</h2><pre>")
		for _, p := range online {
			_, _ = fmt.Fprintf(w, "%s (%s) Gold:%d Rep:%d\n", p.Name, reputationTitle(p.Rep), p.Gold, p.Rep)
		}
		_, _ = fmt.Fprintf(w, "</pre></body></html>")
	})

	mux.HandleFunc("/admin/tick", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !isAdmin(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		store.mu.Lock()
		runWorldTickLocked(store, time.Now().UTC())
		store.mu.Unlock()
		http.Redirect(w, r, "/admin"+adminTokenSuffix(r), http.StatusSeeOther)
	})

	mux.HandleFunc("/admin/reset", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !isAdmin(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		store.mu.Lock()
		resetStoreLocked(store)
		store.mu.Unlock()
		http.Redirect(w, r, "/admin"+adminTokenSuffix(r), http.StatusSeeOther)
	})
	return mux
}

func parseTemplates() *template.Template {
	base := template.Must(template.New("root").ParseGlob(filepath.Join(templateRoot, "*.html")))
	return template.Must(base.ParseGlob(filepath.Join(templateRoot, "fragments", "*.html")))
}

func newStore() *Store {
	now := time.Now().UTC()
	s := &Store{
		World: WorldState{
			DayNumber:              1,
			Subphase:               "Morning",
			GrainSupply:            300,
			GrainTier:              "Stable",
			UnrestValue:            5,
			UnrestTier:             "Calm",
			RestrictedMarketsTicks: 0,
			Situation:              deriveSituation("Stable", "Calm"),
		},
		Players:           map[string]*Player{},
		Contracts:         map[string]*Contract{},
		Institutions:      map[string]*Institution{},
		Seats:             map[string]*Seat{},
		Policies:          PolicyState{TaxRatePct: 0},
		Rumors:            map[int64]*Rumor{},
		Evidence:          map[int64]*Evidence{},
		Loans:             map[string]*Loan{},
		Obligations:       map[string]*Obligation{},
		Events:            []Event{},
		Chat:              []ChatMessage{},
		LastDailyTickDate: "",
		LastTickAt:        now,
		TickEvery:         60 * time.Second,
		LastChatAt:        map[string]time.Time{},
		LastActionAt:      map[string]time.Time{},
		LastDeliverAt:     map[string]time.Time{},
		LastInvestigateAt: map[string]int64{},
		LastSeatActionAt:  map[string]int64{},
		LastIntelActionAt: map[string]int64{},
		DailyActionDate:   map[string]string{},
		DailyHighImpactN:  map[string]int{},
		ToastByPlayer:     map[string]string{},
		rng:               mathrand.New(mathrand.NewSource(now.UnixNano())),
	}
	initializeInstitutionsLocked(s)
	addEventLocked(s, Event{Type: "Opening", Severity: 1, Text: "The granary gates creak open under a restless sky.", At: now})
	return s
}

func resetStoreLocked(s *Store) {
	now := time.Now().UTC()
	s.World = WorldState{
		DayNumber:              1,
		Subphase:               "Morning",
		GrainSupply:            300,
		GrainTier:              "Stable",
		UnrestValue:            5,
		UnrestTier:             "Calm",
		RestrictedMarketsTicks: 0,
		Situation:              deriveSituation("Stable", "Calm"),
	}
	s.Players = map[string]*Player{}
	s.Contracts = map[string]*Contract{}
	s.Institutions = map[string]*Institution{}
	s.Seats = map[string]*Seat{}
	s.Policies = PolicyState{TaxRatePct: 0}
	s.Rumors = map[int64]*Rumor{}
	s.Evidence = map[int64]*Evidence{}
	s.Loans = map[string]*Loan{}
	s.Obligations = map[string]*Obligation{}
	s.Events = []Event{}
	s.Chat = []ChatMessage{}
	s.NextEventID = 0
	s.NextContractID = 0
	s.NextChatID = 0
	s.LastDailyTickDate = ""
	s.LastTickAt = now
	s.TickCount = 0
	s.LastChatAt = map[string]time.Time{}
	s.LastActionAt = map[string]time.Time{}
	s.LastDeliverAt = map[string]time.Time{}
	s.LastInvestigateAt = map[string]int64{}
	s.LastSeatActionAt = map[string]int64{}
	s.LastIntelActionAt = map[string]int64{}
	s.DailyActionDate = map[string]string{}
	s.DailyHighImpactN = map[string]int{}
	s.ToastByPlayer = map[string]string{}
	initializeInstitutionsLocked(s)
	addEventLocked(s, Event{Type: "Reset", Severity: 1, Text: "The test realm is reset; old deals and names are gone.", At: now})
}

// Tick scheduler: checks every second, advances at fixed cadence only if someone is online.
// This prevents per-player action time acceleration in the shared world.
func startTickScheduler(store *Store) {
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for now := range ticker.C {
			store.mu.Lock()
			online := len(onlinePlayersLocked(store, now.UTC())) > 0
			if online && now.UTC().Sub(store.LastTickAt) >= store.TickEvery {
				runWorldTickLocked(store, now.UTC())
				store.LastTickAt = now.UTC()
			}
			store.mu.Unlock()
		}
	}()
}

func runWorldTickLocked(store *Store, now time.Time) {
	store.TickCount++
	processInstitutionTickLocked(store, now)
	processIntelTickLocked(store, now)
	processFinanceTickLocked(store, now)
	processPlayerTickLocked(store)
	w := &store.World
	prevGrainTier := w.GrainTier
	prevUnrestTier := w.UnrestTier

	if w.Subphase == "Morning" {
		w.Subphase = "Evening"
	} else {
		w.Subphase = "Morning"
		w.DayNumber++
	}

	if w.RestrictedMarketsTicks > 0 {
		w.RestrictedMarketsTicks--
	}

	w.GrainSupply -= 18 + store.rng.Intn(9)
	if w.GrainSupply < 0 {
		w.GrainSupply = 0
	}
	if store.rng.Float64() < 0.10 {
		w.GrainSupply -= 25
		if w.GrainSupply < 0 {
			w.GrainSupply = 0
		}
	}
	if store.rng.Float64() < 0.08 && grainTierFromSupply(w.GrainSupply) != "Stable" {
		w.GrainSupply += 20
	}

	w.GrainTier = grainTierFromSupply(w.GrainSupply)

	fulfilledThisTick := 0
	failedThisTick := 0

	for _, c := range sortedContractsLocked(store) {
		if c.Status != "Issued" && c.Status != "Accepted" && c.Status != "Ignored" {
			continue
		}

		if c.Status == "Accepted" && c.OwnerPlayerID != "" {
			owner := store.Players[c.OwnerPlayerID]
			if owner != nil && now.Sub(owner.LastSeen) > inactiveWindow {
				if c.DeadlineTicks <= 1 {
					c.Status = "Failed"
					failedThisTick++
					applyFailurePenaltyLocked(store, owner)
					addEventLocked(store, Event{Type: "Contract", Severity: 3, Text: "A contractor vanishes and the deal collapses.", At: now})
				} else {
					c.Status = "Issued"
					c.OwnerPlayerID = ""
					c.OwnerName = ""
					c.Stance = ""
					addEventLocked(store, Event{Type: "Contract", Severity: 2, Text: "A contractor vanishes; the job returns to the board.", At: now})
				}
				continue
			}
		}

		chance := fulfillChanceForTier(w.GrainTier)
		if c.Status == "Accepted" {
			chance = minInt(chance+15, 95)
		}
		if rollPercent(store.rng, chance) {
			c.Status = "Fulfilled"
			grainReward := 30
			if c.Type == "Emergency" {
				grainReward = 60
			}
			w.GrainSupply += grainReward
			w.GrainTier = grainTierFromSupply(w.GrainSupply)
			fulfilledThisTick++
			addEventLocked(store, Event{Type: "Contract", Severity: 2, Text: "A contract lands successfully despite the strain.", At: now})
			continue
		}

		c.DeadlineTicks--
		if c.DeadlineTicks <= 0 {
			c.Status = "Failed"
			failedThisTick++
			if c.OwnerPlayerID != "" {
				if owner := store.Players[c.OwnerPlayerID]; owner != nil {
					applyFailurePenaltyLocked(store, owner)
				}
			}
			addEventLocked(store, Event{Type: "Contract", Severity: 3, Text: "A contract has failed, raising tension in the city.", At: now})
		}
	}

	baseMultiplier := map[string]float64{"Stable": 1.0, "Tight": 1.5, "Scarce": 2.0, "Critical": 3.0}[w.GrainTier]
	effectiveMultiplier := baseMultiplier
	if w.RestrictedMarketsTicks > 0 {
		effectiveMultiplier = maxFloat(1.0, baseMultiplier-0.5)
	}

	if effectiveMultiplier >= 2.0 {
		w.UnrestValue += 5
	}
	w.UnrestValue += failedThisTick * 15
	w.UnrestValue -= fulfilledThisTick * 10

	if w.GrainTier == "Critical" {
		w.CriticalTickStreak++
		if w.CriticalTickStreak >= 4 && !w.CriticalStreakPenaltyApplied {
			w.UnrestValue += 10
			w.CriticalStreakPenaltyApplied = true
		}
	} else {
		w.CriticalTickStreak = 0
		w.CriticalStreakPenaltyApplied = false
	}

	w.UnrestValue = clampInt(w.UnrestValue, 0, 100)
	w.UnrestTier = unrestTierFromValue(w.UnrestValue)

	if (w.UnrestTier == "Unstable" || w.UnrestTier == "Rioting" || w.GrainTier == "Critical") && !hasActiveContractLocked(store, "Emergency") {
		issueContractLocked(store, "Emergency", 4)
		addEventLocked(store, Event{Type: "Faction", Severity: 3, Text: "[City Authority] requisitions emergency shipments.", At: now})
	}
	if w.UnrestTier == "Rioting" && w.RestrictedMarketsTicks == 0 {
		w.RestrictedMarketsTicks = 2
		addEventLocked(store, Event{Type: "Faction", Severity: 4, Text: "[City Authority] imposes strict market controls.", At: now})
	}
	if (w.GrainTier == "Scarce" || w.GrainTier == "Critical") && !hasActiveContractLocked(store, "Smuggling") {
		issueContractLocked(store, "Smuggling", 3)
		addEventLocked(store, Event{Type: "Faction", Severity: 3, Text: "[Merchant League] issues smuggling orders.", At: now})
	}

	if w.GrainTier != prevGrainTier {
		addEventLocked(store, Event{Type: "Grain", Severity: 2, Text: grainTierNarrative(prevGrainTier, w.GrainTier), At: now})
	}
	if w.UnrestTier != prevUnrestTier {
		addEventLocked(store, Event{Type: "Unrest", Severity: 3, Text: unrestTierNarrative(prevUnrestTier, w.UnrestTier), At: now})
	}

	w.Situation = deriveSituation(w.GrainTier, w.UnrestTier)
	if !addedTickNarrative(now, store.Events) {
		if store.rng.Intn(100) < 15 {
			addEventLocked(store, Event{Type: "Atmosphere", Severity: 1, Text: "Lantern light flickers as rumors outrun the truth.", At: now})
		}
	}
}

func addedTickNarrative(now time.Time, events []Event) bool {
	if len(events) == 0 {
		return false
	}
	last := events[len(events)-1]
	return last.At.Equal(now)
}

func initializeInstitutionsLocked(store *Store) {
	store.Institutions["city_authority"] = &Institution{ID: "city_authority", Name: "City Authority"}
	store.Institutions["merchant_league"] = &Institution{ID: "merchant_league", Name: "Merchant League"}
	store.Institutions["temple"] = &Institution{ID: "temple", Name: "Temple"}

	store.Seats["harbor_master"] = &Seat{
		ID:              "harbor_master",
		Name:            "Harbor Master",
		InstitutionID:   "merchant_league",
		HolderName:      "Captain Vey (NPC)",
		TenureTicksLeft: seatTenureTicks,
	}
	store.Seats["master_of_coin"] = &Seat{
		ID:              "master_of_coin",
		Name:            "Master of Coin",
		InstitutionID:   "city_authority",
		HolderName:      "Clerk Marn (NPC)",
		TenureTicksLeft: seatTenureTicks,
	}
	store.Seats["high_curate"] = &Seat{
		ID:              "high_curate",
		Name:            "High Curate",
		InstitutionID:   "temple",
		HolderName:      "Sister Hal (NPC)",
		TenureTicksLeft: seatTenureTicks,
	}
}

func processInstitutionTickLocked(store *Store, now time.Time) {
	if store.Policies.SmugglingEmbargoTicks > 0 {
		store.Policies.SmugglingEmbargoTicks--
		if store.Policies.SmugglingEmbargoTicks == 0 {
			addEventLocked(store, Event{Type: "Policy", Severity: 1, Text: "Smuggling embargo expires.", At: now})
		}
	}
	for _, seat := range store.Seats {
		if seat.ElectionWindowTicks > 0 {
			seat.ElectionWindowTicks--
			if seat.ElectionWindowTicks == 0 {
				resolveElectionLocked(store, seat, now)
			}
			continue
		}
		seat.TenureTicksLeft--
		if seat.TenureTicksLeft <= 0 {
			seat.ElectionWindowTicks = electionWindowTicks
			seat.TenureTicksLeft = 0
			addEventLocked(store, Event{
				Type:     "Institution",
				Severity: 2,
				Text:     fmt.Sprintf("Election opens for %s.", seat.Name),
				At:       now,
			})
		}
	}
}

func resolveElectionLocked(store *Store, seat *Seat, now time.Time) {
	var winner *Player
	for _, p := range store.Players {
		if winner == nil || p.Rep > winner.Rep || (p.Rep == winner.Rep && p.Name < winner.Name) {
			winner = p
		}
	}
	if winner == nil {
		seat.HolderPlayerID = ""
		seat.HolderName = seatDefaultHolderName(seat.ID)
		seat.TenureTicksLeft = seatTenureTicks
		addEventLocked(store, Event{
			Type:     "Institution",
			Severity: 1,
			Text:     fmt.Sprintf("%s returns to appointment by default.", seat.Name),
			At:       now,
		})
		return
	}
	seat.HolderPlayerID = winner.ID
	seat.HolderName = winner.Name
	seat.TenureTicksLeft = seatTenureTicks
	addEventLocked(store, Event{
		Type:     "Institution",
		Severity: 2,
		Text:     fmt.Sprintf("[%s] secures election as %s.", winner.Name, seat.Name),
		At:       now,
	})
}

func seatDefaultHolderName(seatID string) string {
	switch seatID {
	case "harbor_master":
		return "Captain Vey (NPC)"
	case "master_of_coin":
		return "Clerk Marn (NPC)"
	case "high_curate":
		return "Sister Hal (NPC)"
	default:
		return "Appointee (NPC)"
	}
}

func playerHoldsSeatLocked(store *Store, playerID, seatID string) bool {
	seat := store.Seats[seatID]
	return seat != nil && seat.HolderPlayerID == playerID
}

func processIntelTickLocked(store *Store, now time.Time) {
	for id, r := range store.Rumors {
		r.Spread += maxInt(1, r.Credibility/3)
		r.Decay--
		if r.Spread >= 6 {
			if target := store.Players[r.TargetPlayerID]; target != nil {
				target.Rep = clampInt(target.Rep-1, -100, 100)
				target.Heat = clampInt(target.Heat+1, 0, 20)
			}
		}
		if r.Decay <= 0 {
			delete(store.Rumors, id)
		}
	}

	for id, ev := range store.Evidence {
		if ev.ExpiryTick <= store.TickCount {
			delete(store.Evidence, id)
		}
	}
}

func processFinanceTickLocked(store *Store, now time.Time) {
	defaultsThisTick := 0
	for _, loan := range store.Loans {
		if loan.Status == "Active" && loan.DueTick <= store.TickCount && loan.Remaining > 0 {
			processLoanDefaultLocked(store, loan, now)
			defaultsThisTick++
		}
	}
	if defaultsThisTick > 0 {
		store.World.UnrestValue = clampInt(store.World.UnrestValue+defaultsThisTick*2, 0, 100)
		store.World.UnrestTier = unrestTierFromValue(store.World.UnrestValue)
	}
}

func processPlayerTickLocked(store *Store) {
	for _, p := range store.Players {
		if p.RiteImmunityTicks > 0 {
			p.RiteImmunityTicks--
		}
	}
}

func addRumorLocked(store *Store, r *Rumor, now time.Time) {
	store.NextRumorID++
	r.ID = store.NextRumorID
	store.Rumors[r.ID] = r
	addEventLocked(store, Event{
		Type:     "Intel",
		Severity: 2,
		Text:     fmt.Sprintf("[%s] seeds a rumor about [%s].", r.SourceName, r.TargetName),
		At:       now,
	})
}

func addEvidenceLocked(store *Store, source *Player, target *Player, topic string, strength int, ttlTicks int64) {
	if source == nil || target == nil {
		return
	}
	store.NextEvidenceID++
	id := store.NextEvidenceID
	store.Evidence[id] = &Evidence{
		ID:             id,
		Topic:          topic,
		TargetPlayerID: target.ID,
		TargetName:     target.Name,
		SourcePlayerID: source.ID,
		SourceName:     source.Name,
		Strength:       clampInt(strength, 1, 10),
		ExpiryTick:     store.TickCount + ttlTicks,
	}
}

func strongestEvidenceForLocked(store *Store, sourceID, targetID string) *Evidence {
	var out *Evidence
	for _, ev := range store.Evidence {
		if ev.SourcePlayerID != sourceID || ev.TargetPlayerID != targetID {
			continue
		}
		if out == nil || ev.Strength > out.Strength {
			out = ev
		}
	}
	return out
}

func addObligationLocked(store *Store, creditor, debtor *Player, reason string, severity int) {
	if creditor == nil || debtor == nil {
		return
	}
	store.NextObligationID++
	id := fmt.Sprintf("o-%d", store.NextObligationID)
	store.Obligations[id] = &Obligation{
		ID:               id,
		CreditorPlayerID: creditor.ID,
		CreditorName:     creditor.Name,
		DebtorPlayerID:   debtor.ID,
		DebtorName:       debtor.Name,
		Reason:           reason,
		Severity:         clampInt(severity, 1, 5),
		DueTick:          store.TickCount + 3,
		Status:           "Open",
	}
}

func triggerInstitutionSanctionLocked(store *Store, target *Player, now time.Time) {
	if target == nil {
		return
	}
	target.Heat = clampInt(target.Heat+3, 0, 20)
	store.Policies.PermitRequiredHighRisk = true
	if seat := store.Seats["harbor_master"]; seat != nil && seat.HolderPlayerID == target.ID {
		seat.HolderPlayerID = ""
		seat.HolderName = seatDefaultHolderName(seat.ID)
		seat.TenureTicksLeft = seatTenureTicks
	}
	addEventLocked(store, Event{
		Type:     "Institution",
		Severity: 4,
		Text:     fmt.Sprintf("Institutional inquiry sanctions [%s].", target.Name),
		At:       now,
	})
}

func processLoanDefaultLocked(store *Store, loan *Loan, now time.Time) {
	if loan == nil || loan.Status != "Active" {
		return
	}
	loan.Status = "Defaulted"
	borrower := store.Players[loan.BorrowerPlayerID]
	lender := store.Players[loan.LenderPlayerID]
	if borrower != nil {
		borrower.Rep = clampInt(borrower.Rep-6, -100, 100)
		borrower.Heat = clampInt(borrower.Heat+2, 0, 20)
	}
	if lender != nil {
		lender.Rep = clampInt(lender.Rep-1, -100, 100)
	}
	store.Policies.SmugglingEmbargoTicks = maxInt(store.Policies.SmugglingEmbargoTicks, 2)
	addEventLocked(store, Event{
		Type:     "Finance",
		Severity: 4,
		Text:     fmt.Sprintf("Loan default by [%s] triggers sanctions and market fear.", loan.BorrowerName),
		At:       now,
	})
}

func consumeHighImpactBudgetLocked(store *Store, playerID string, now time.Time) bool {
	today := now.UTC().Format("2006-01-02")
	if store.DailyActionDate[playerID] != today {
		store.DailyActionDate[playerID] = today
		store.DailyHighImpactN[playerID] = 0
	}
	if store.DailyHighImpactN[playerID] >= highImpactDailyCap {
		return false
	}
	store.DailyHighImpactN[playerID]++
	return true
}

func tooSoonTick(last, now int64, cooldown int64) bool {
	if last == 0 {
		return false
	}
	return now-last < cooldown
}

func chooseTopic(given, fallback string) string {
	if strings.TrimSpace(given) != "" {
		return strings.TrimSpace(given)
	}
	return fallback
}

type ActionInput struct {
	Action     string
	ContractID string
	Stance     string
	TargetID   string
	Claim      string
	Topic      string
	LoanID     string
	Amount     int
}

func handleActionLocked(store *Store, p *Player, now time.Time, action, contractID string, stanceInput ...string) {
	in := ActionInput{
		Action:     action,
		ContractID: contractID,
	}
	if len(stanceInput) > 0 {
		in.Stance = stanceInput[0]
	}
	handleActionInputLocked(store, p, now, in)
}

func handleActionInputLocked(store *Store, p *Player, now time.Time, in ActionInput) {
	action := in.Action
	contractID := in.ContractID
	stance := in.Stance
	c := store.Contracts[contractID]

	// Actions mutate local/player/contract state but never advance world time.
	// Time progression is owned by fixed scheduler ticks for fair multi-player simulation.
	switch action {
	case "accept":
		if c == nil {
			setToastLocked(store, p.ID, "That contract is unavailable.")
			return
		}
		if c.Status == "Accepted" {
			owner := c.OwnerName
			if owner == "" {
				owner = "another contractor"
			}
			setToastLocked(store, p.ID, fmt.Sprintf("Taken by %s.", owner))
			return
		}
		if c.Status != "Issued" {
			setToastLocked(store, p.ID, "That contract is unavailable.")
			return
		}
		if c.Type == "Smuggling" && p.Rep < -50 {
			setToastLocked(store, p.ID, "Your reputation blocks smuggling contracts.")
			return
		}
		if c.Type == "Smuggling" && store.Policies.SmugglingEmbargoTicks > 0 && !playerHoldsSeatLocked(store, p.ID, "harbor_master") {
			setToastLocked(store, p.ID, "Smuggling is under embargo.")
			return
		}
		if c.Type == "Emergency" && store.Policies.PermitRequiredHighRisk && p.Rep < 20 && !playerHoldsSeatLocked(store, p.ID, "harbor_master") {
			setToastLocked(store, p.ID, "Permit required for emergency contracts.")
			return
		}
		if playerAcceptedCountLocked(store, p.ID) >= 1 {
			setToastLocked(store, p.ID, "You can hold only one active contract.")
			return
		}
		c.Status = "Accepted"
		c.OwnerPlayerID = p.ID
		c.OwnerName = p.Name
		c.Stance = normalizeContractStance(stance)
		addEventLocked(store, Event{Type: "Player", Severity: 2, Text: fmt.Sprintf("[%s] commits to a dangerous contract.", p.Name), At: now})
		setToastLocked(store, p.ID, "Contract accepted.")
	case "ignore":
		if c == nil || c.Status != "Issued" {
			setToastLocked(store, p.ID, "Nothing to ignore here.")
			return
		}
		c.Status = "Ignored"
		addEventLocked(store, Event{Type: "Player", Severity: 1, Text: fmt.Sprintf("[%s] turns away as pressure mounts.", p.Name), At: now})
		setToastLocked(store, p.ID, "Ignored.")
	case "abandon":
		if c == nil || c.Status != "Accepted" || c.OwnerPlayerID != p.ID {
			setToastLocked(store, p.ID, "You can only abandon your own accepted contract.")
			return
		}
		p.Rep = clampInt(p.Rep-2, -100, 100)
		c.Status = "Issued"
		c.OwnerPlayerID = ""
		c.OwnerName = ""
		c.Stance = ""
		addEventLocked(store, Event{Type: "Player", Severity: 2, Text: fmt.Sprintf("[%s] abandons a claim as the city watches.", p.Name), At: now})
		addEventLocked(store, Event{Type: "Consequence", Severity: 1, Text: "Word spreads: your reputation in Black Granary shifts.", At: now})
		setToastLocked(store, p.ID, "Contract abandoned.")
	case "deliver":
		if c == nil || c.OwnerPlayerID != p.ID || (c.Status != "Accepted" && c.Status != "Fulfilled") {
			setToastLocked(store, p.ID, "You can only deliver your accepted or fulfilled contract.")
			return
		}
		if c.Status == "Fulfilled" {
			finalizeDeliveredContractLocked(store, p, c, now)
			setToastLocked(store, p.ID, "Delivery completed.")
			return
		}
		if c.Status == "Accepted" {
			if p.Gold < 2 {
				setToastLocked(store, p.ID, "You need 2g to attempt a delivery.")
				return
			}
			if tooSoon(store.LastDeliverAt[p.ID], now, deliverCooldown) {
				setToastLocked(store, p.ID, "Delivery cooldown active.")
				return
			}
			store.LastDeliverAt[p.ID] = now
			p.Gold = maxInt(0, p.Gold-2)

			chance := deliverChanceByTier(store.World.GrainTier)
			if rollPercent(store.rng, chance) {
				finalizeDeliveredContractLocked(store, p, c, now)
				setToastLocked(store, p.ID, "Delivery succeeded.")
			} else {
				p.Rep = clampInt(p.Rep-5, -100, 100)
				addEventLocked(store, Event{Type: "Player", Severity: 2, Text: fmt.Sprintf("[%s] attempts a delivery, but it collapses at the last moment.", p.Name), At: now})
				addEventLocked(store, Event{Type: "Consequence", Severity: 1, Text: "Word spreads: your reputation in Black Granary shifts.", At: now})
				setToastLocked(store, p.ID, "Delivery failed.")
			}
			return
		}
	case "investigate", "investigate_target":
		lastTick, ok := store.LastInvestigateAt[p.ID]
		if !ok || store.TickCount-lastTick >= 3 {
			store.LastInvestigateAt[p.ID] = store.TickCount
			store.World.UnrestValue = clampInt(store.World.UnrestValue-5, 0, 100)
			store.World.UnrestTier = unrestTierFromValue(store.World.UnrestValue)
			p.Rep = clampInt(p.Rep+1, -100, 100)
			p.Rumors += rumorInvestigateGain
			addEventLocked(store, Event{Type: "Player", Severity: 2, Text: fmt.Sprintf("[%s] investigates rumors along the supply routes.", p.Name), At: now})
			if in.TargetID != "" {
				if target := store.Players[in.TargetID]; target != nil && target.ID != p.ID {
					addEvidenceLocked(store, p, target, chooseTopic(in.Topic, "corruption"), 5+maxInt(0, p.Rep/25), 5)
					setToastLocked(store, p.ID, "Your investigation found evidence.")
					break
				}
			}
			setToastLocked(store, p.ID, "Your investigation calmed the streets.")
		} else {
			addEventLocked(store, Event{Type: "Player", Severity: 1, Text: fmt.Sprintf("[%s] investigates rumors along the supply routes.", p.Name), At: now})
			setToastLocked(store, p.ID, "You find only fragments and gossip.")
		}
	case "seed_rumor":
		target := store.Players[in.TargetID]
		if target == nil || target.ID == p.ID {
			setToastLocked(store, p.ID, "Choose a valid target for rumor seeding.")
			return
		}
		if tooSoonTick(store.LastIntelActionAt[p.ID], store.TickCount, 1) {
			setToastLocked(store, p.ID, "Rumor operation cooldown active.")
			return
		}
		store.LastIntelActionAt[p.ID] = store.TickCount
		claim := in.Claim
		if claim == "" {
			claim = fmt.Sprintf("%s diverted relief grain.", target.Name)
		}
		addRumorLocked(store, &Rumor{
			Claim:          claim,
			Topic:          chooseTopic(in.Topic, "corruption"),
			TargetPlayerID: target.ID,
			TargetName:     target.Name,
			SourcePlayerID: p.ID,
			SourceName:     p.Name,
			Credibility:    clampInt(4+p.Rep/20, 1, 9),
			Spread:         2,
			Decay:          5,
		}, now)
		p.Heat = clampInt(p.Heat+1, 0, 20)
		setToastLocked(store, p.ID, "Rumor seeded.")
	case "publish_evidence":
		target := store.Players[in.TargetID]
		if target == nil || target.ID == p.ID {
			setToastLocked(store, p.ID, "Choose a valid target to publish evidence.")
			return
		}
		if !consumeHighImpactBudgetLocked(store, p.ID, now) {
			setToastLocked(store, p.ID, "Daily cap reached for high-impact actions.")
			return
		}
		ev := strongestEvidenceForLocked(store, p.ID, target.ID)
		if ev == nil {
			setToastLocked(store, p.ID, "You lack evidence on that target.")
			return
		}
		delete(store.Evidence, ev.ID)
		target.Rep = clampInt(target.Rep-ev.Strength, -100, 100)
		target.Heat = clampInt(target.Heat+ev.Strength/2+1, 0, 20)
		p.Rep = clampInt(p.Rep+2, -100, 100)
		addEventLocked(store, Event{
			Type:     "Intel",
			Severity: 3,
			Text:     fmt.Sprintf("[%s] publishes evidence against [%s].", p.Name, target.Name),
			At:       now,
		})
		if ev.Strength >= 6 {
			triggerInstitutionSanctionLocked(store, target, now)
		}
		setToastLocked(store, p.ID, "Evidence published.")
	case "counter_narrative":
		target := store.Players[in.TargetID]
		if target == nil {
			setToastLocked(store, p.ID, "Choose a rumor target to counter.")
			return
		}
		changed := false
		for _, r := range store.Rumors {
			if r.TargetPlayerID != target.ID {
				continue
			}
			r.Spread = maxInt(0, r.Spread-2)
			r.Decay = maxInt(0, r.Decay-2)
			changed = true
		}
		if changed {
			p.Rep = clampInt(p.Rep+1, -100, 100)
			setToastLocked(store, p.ID, "Counter-narrative slows rumor spread.")
		} else {
			setToastLocked(store, p.ID, "No major rumor wave found to counter.")
		}
	case "loan_offer":
		target := store.Players[in.TargetID]
		principal := clampInt(in.Amount, 1, 1000)
		if target == nil || target.ID == p.ID {
			setToastLocked(store, p.ID, "Choose a borrower.")
			return
		}
		if principal <= 0 || p.Gold < principal {
			setToastLocked(store, p.ID, "Insufficient gold to issue loan.")
			return
		}
		store.NextLoanID++
		id := fmt.Sprintf("l-%d", store.NextLoanID)
		store.Loans[id] = &Loan{
			ID:               id,
			LenderPlayerID:   p.ID,
			LenderName:       p.Name,
			BorrowerPlayerID: target.ID,
			BorrowerName:     target.Name,
			Principal:        principal,
			Remaining:        principal,
			DueTick:          store.TickCount + loanDueTicks,
			Status:           "Offered",
		}
		addEventLocked(store, Event{Type: "Finance", Severity: 2, Text: fmt.Sprintf("[%s] offers a loan to [%s].", p.Name, target.Name), At: now})
		setToastLocked(store, p.ID, "Loan offer issued.")
	case "loan_accept":
		loan := store.Loans[in.LoanID]
		if loan == nil || loan.Status != "Offered" || loan.BorrowerPlayerID != p.ID {
			setToastLocked(store, p.ID, "No matching loan offer.")
			return
		}
		lender := store.Players[loan.LenderPlayerID]
		if lender == nil || lender.Gold < loan.Principal {
			setToastLocked(store, p.ID, "Lender cannot fund this loan.")
			loan.Status = "Cancelled"
			return
		}
		lender.Gold -= loan.Principal
		p.Gold += loan.Principal
		loan.Status = "Active"
		addEventLocked(store, Event{Type: "Finance", Severity: 2, Text: fmt.Sprintf("[%s] accepts credit from [%s].", p.Name, lender.Name), At: now})
		setToastLocked(store, p.ID, "Loan accepted.")
	case "repay":
		loan := store.Loans[in.LoanID]
		if loan == nil || loan.Status != "Active" || loan.BorrowerPlayerID != p.ID {
			setToastLocked(store, p.ID, "No active loan to repay.")
			return
		}
		amount := in.Amount
		if amount <= 0 || amount > loan.Remaining {
			amount = loan.Remaining
		}
		if p.Gold < amount {
			setToastLocked(store, p.ID, "Not enough gold to repay.")
			return
		}
		lender := store.Players[loan.LenderPlayerID]
		p.Gold -= amount
		loan.Remaining -= amount
		if lender != nil {
			lender.Gold += amount
			lender.Rep = clampInt(lender.Rep+1, -100, 100)
		}
		if loan.Remaining == 0 {
			loan.Status = "Repaid"
			p.Rep = clampInt(p.Rep+2, -100, 100)
			addEventLocked(store, Event{Type: "Finance", Severity: 2, Text: fmt.Sprintf("[%s] repays debt to [%s].", p.Name, loan.LenderName), At: now})
		}
		setToastLocked(store, p.ID, "Loan repayment processed.")
	case "default":
		loan := store.Loans[in.LoanID]
		if loan == nil || loan.Status != "Active" || loan.BorrowerPlayerID != p.ID {
			setToastLocked(store, p.ID, "No active loan to default.")
			return
		}
		processLoanDefaultLocked(store, loan, now)
		setToastLocked(store, p.ID, "You defaulted on the loan.")
	case "bribe_official":
		targetSeat := store.Seats["harbor_master"]
		cost := maxInt(2, in.Amount)
		if p.Gold < cost {
			setToastLocked(store, p.ID, "You cannot afford the bribe.")
			return
		}
		p.Gold -= cost
		p.Heat = clampInt(p.Heat+2, 0, 20)
		if targetSeat != nil && targetSeat.HolderPlayerID != "" && targetSeat.HolderPlayerID != p.ID {
			p.Rep = clampInt(p.Rep+1, -100, 100)
		}
		addEventLocked(store, Event{Type: "Institution", Severity: 3, Text: fmt.Sprintf("[%s] bribes officials for temporary access.", p.Name), At: now})
		setToastLocked(store, p.ID, "Bribe executed.")
	case "petition_institution":
		if p.Rep >= 10 {
			p.Heat = maxInt(0, p.Heat-1)
			store.World.UnrestValue = maxInt(0, store.World.UnrestValue-2)
			store.World.UnrestTier = unrestTierFromValue(store.World.UnrestValue)
			setToastLocked(store, p.ID, "Your petition is heard.")
		} else {
			p.Rep = clampInt(p.Rep-1, -100, 100)
			setToastLocked(store, p.ID, "Your petition is ignored.")
		}
	case "threaten_exposure":
		target := store.Players[in.TargetID]
		if target == nil || target.ID == p.ID {
			setToastLocked(store, p.ID, "Choose a valid target.")
			return
		}
		if !consumeHighImpactBudgetLocked(store, p.ID, now) {
			setToastLocked(store, p.ID, "Daily cap reached for high-impact actions.")
			return
		}
		ev := strongestEvidenceForLocked(store, p.ID, target.ID)
		if ev == nil {
			setToastLocked(store, p.ID, "You need evidence before threatening exposure.")
			return
		}
		payout := minInt(6, maxInt(2, target.Gold/3))
		if payout > 0 {
			target.Gold -= payout
			p.Gold += payout
			addObligationLocked(store, p, target, "silence payment", 2)
			setToastLocked(store, p.ID, "Exposure threat forces a concession.")
		} else {
			target.Rep = clampInt(target.Rep-3, -100, 100)
			setToastLocked(store, p.ID, "Target cannot pay; reputation damage lands instead.")
		}
	case "broker_deal":
		if p.Gold < 2 {
			setToastLocked(store, p.ID, "Need 2g to broker a deal.")
			return
		}
		p.Gold -= 2
		boosted := false
		for _, c := range store.Contracts {
			if c.Status == "Issued" {
				c.DeadlineTicks++
				boosted = true
				break
			}
		}
		if boosted {
			p.Rep = clampInt(p.Rep+1, -100, 100)
			addEventLocked(store, Event{Type: "Player", Severity: 2, Text: fmt.Sprintf("[%s] brokers a multi-party deal to stabilize routes.", p.Name), At: now})
			setToastLocked(store, p.ID, "Deal brokered; contract pressure eased.")
		} else {
			setToastLocked(store, p.ID, "No contract available to broker.")
		}
	case "invoke_rite":
		p.RiteImmunityTicks = 3
		p.Rep = clampInt(p.Rep+2, -100, 100)
		addEventLocked(store, Event{Type: "Doctrine", Severity: 2, Text: fmt.Sprintf("[%s] invokes rite and claims moral protection.", p.Name), At: now})
		setToastLocked(store, p.ID, "Rite invoked: temporary inquiry immunity.")
	case "accuse_heresy":
		target := store.Players[in.TargetID]
		if target == nil || target.ID == p.ID {
			setToastLocked(store, p.ID, "Choose a valid target.")
			return
		}
		if !consumeHighImpactBudgetLocked(store, p.ID, now) {
			setToastLocked(store, p.ID, "Daily cap reached for high-impact actions.")
			return
		}
		if target.RiteImmunityTicks > 0 {
			p.Rep = clampInt(p.Rep-2, -100, 100)
			setToastLocked(store, p.ID, "Ritual immunity blunts your accusation.")
			return
		}
		successChance := 35 + maxInt(0, p.Rep)/3
		if rollPercent(store.rng, minInt(successChance, 85)) {
			target.Rep = clampInt(target.Rep-6, -100, 100)
			target.Heat = clampInt(target.Heat+2, 0, 20)
			p.Rep = clampInt(p.Rep+1, -100, 100)
			addEventLocked(store, Event{Type: "Doctrine", Severity: 3, Text: fmt.Sprintf("[%s] accuses [%s] of heresy; crowds demand inquiry.", p.Name, target.Name), At: now})
			setToastLocked(store, p.ID, "Accusation gains traction.")
		} else {
			p.Rep = clampInt(p.Rep-3, -100, 100)
			setToastLocked(store, p.ID, "Your accusation backfires.")
		}
	case "campaign_seat":
		seat := store.Seats[contractID]
		if seat == nil || seat.ElectionWindowTicks <= 0 {
			setToastLocked(store, p.ID, "No election is open for that seat.")
			return
		}
		seat.HolderPlayerID = p.ID
		seat.HolderName = p.Name
		seat.ElectionWindowTicks = 0
		seat.TenureTicksLeft = seatTenureTicks
		addEventLocked(store, Event{
			Type:     "Institution",
			Severity: 2,
			Text:     fmt.Sprintf("[%s] wins the seat of %s.", p.Name, seat.Name),
			At:       now,
		})
		setToastLocked(store, p.ID, fmt.Sprintf("You now hold %s.", seat.Name))
	case "challenge_seat":
		seat := store.Seats[contractID]
		if seat == nil {
			setToastLocked(store, p.ID, "Seat not found.")
			return
		}
		lastTick, ok := store.LastSeatActionAt[p.ID]
		if ok && store.TickCount-lastTick < 2 {
			setToastLocked(store, p.ID, "Institution challenge cooldown active.")
			return
		}
		store.LastSeatActionAt[p.ID] = store.TickCount
		if seat.HolderPlayerID == p.ID {
			setToastLocked(store, p.ID, "You already hold that seat.")
			return
		}
		chance := 30 + maxInt(0, p.Rep)/2
		if rollPercent(store.rng, minInt(chance, 85)) {
			seat.HolderPlayerID = p.ID
			seat.HolderName = p.Name
			seat.ElectionWindowTicks = 0
			seat.TenureTicksLeft = seatTenureTicks
			p.Rep = clampInt(p.Rep+2, -100, 100)
			addEventLocked(store, Event{
				Type:     "Institution",
				Severity: 3,
				Text:     fmt.Sprintf("[%s] forces a successful censure and takes %s.", p.Name, seat.Name),
				At:       now,
			})
			setToastLocked(store, p.ID, "Your censure challenge succeeded.")
		} else {
			p.Rep = clampInt(p.Rep-3, -100, 100)
			addEventLocked(store, Event{
				Type:     "Institution",
				Severity: 2,
				Text:     fmt.Sprintf("[%s] fails to censure the current %s.", p.Name, seat.Name),
				At:       now,
			})
			setToastLocked(store, p.ID, "Your challenge failed and cost political capital.")
		}
	case "set_tax_low":
		if !playerHoldsSeatLocked(store, p.ID, "master_of_coin") {
			setToastLocked(store, p.ID, "Only the Master of Coin can set taxes.")
			return
		}
		store.Policies.TaxRatePct = 5
		addEventLocked(store, Event{Type: "Policy", Severity: 2, Text: fmt.Sprintf("[%s] lowers tax to 5%%.", p.Name), At: now})
		setToastLocked(store, p.ID, "Tax policy updated.")
	case "set_tax_high":
		if !playerHoldsSeatLocked(store, p.ID, "master_of_coin") {
			setToastLocked(store, p.ID, "Only the Master of Coin can set taxes.")
			return
		}
		store.Policies.TaxRatePct = 20
		addEventLocked(store, Event{Type: "Policy", Severity: 3, Text: fmt.Sprintf("[%s] raises tax to 20%%.", p.Name), At: now})
		setToastLocked(store, p.ID, "Tax policy updated.")
	case "toggle_permit":
		if !playerHoldsSeatLocked(store, p.ID, "harbor_master") {
			setToastLocked(store, p.ID, "Only the Harbor Master can control permits.")
			return
		}
		store.Policies.PermitRequiredHighRisk = !store.Policies.PermitRequiredHighRisk
		state := "lifted"
		if store.Policies.PermitRequiredHighRisk {
			state = "required"
		}
		addEventLocked(store, Event{Type: "Policy", Severity: 2, Text: fmt.Sprintf("[%s] marks emergency permits as %s.", p.Name, state), At: now})
		setToastLocked(store, p.ID, "Permit policy updated.")
	case "toggle_embargo":
		if !playerHoldsSeatLocked(store, p.ID, "harbor_master") {
			setToastLocked(store, p.ID, "Only the Harbor Master can set embargoes.")
			return
		}
		if store.Policies.SmugglingEmbargoTicks > 0 {
			store.Policies.SmugglingEmbargoTicks = 0
			addEventLocked(store, Event{Type: "Policy", Severity: 2, Text: fmt.Sprintf("[%s] lifts the smuggling embargo.", p.Name), At: now})
		} else {
			store.Policies.SmugglingEmbargoTicks = 3
			addEventLocked(store, Event{Type: "Policy", Severity: 3, Text: fmt.Sprintf("[%s] imposes a smuggling embargo.", p.Name), At: now})
		}
		setToastLocked(store, p.ID, "Embargo policy updated.")
	default:
		setToastLocked(store, p.ID, "Unknown action.")
	}

	store.World.Situation = deriveSituation(store.World.GrainTier, store.World.UnrestTier)
}

func handleChatLocked(store *Store, p *Player, now time.Time, msg string) bool {
	if strings.HasPrefix(strings.ToLower(msg), "/w ") {
		target, body := resolveWhisperTargetLocked(store, strings.TrimSpace(msg[3:]))
		if body == "" {
			addChatLocked(store, ChatMessage{FromPlayerID: p.ID, FromName: "System", ToPlayerID: p.ID, ToName: p.Name, Text: "Usage: /w <Name> <message>", At: now, Kind: "system"})
			setToastLocked(store, p.ID, "Invalid whisper format.")
			return false
		}
		if target == nil {
			addChatLocked(store, ChatMessage{FromPlayerID: p.ID, FromName: "System", ToPlayerID: p.ID, ToName: p.Name, Text: "Whisper target not found.", At: now, Kind: "system"})
			setToastLocked(store, p.ID, "Player not found.")
			return false
		}
		addChatLocked(store, ChatMessage{FromPlayerID: p.ID, FromName: p.Name, ToPlayerID: target.ID, ToName: target.Name, Text: body, At: now, Kind: "whisper"})
		p.Rumors += rumorWhisperGain
		return true
	}
	addChatLocked(store, ChatMessage{FromPlayerID: p.ID, FromName: p.Name, Text: msg, At: now, Kind: "global"})
	return true
}

func applyFulfillmentRewardsLocked(store *Store, c *Contract) {
	p := store.Players[c.OwnerPlayerID]
	if p == nil {
		return
	}
	baseGold := 25
	repGain := 8
	if c.Type == "Smuggling" {
		baseGold = 35
		repGain = 3
	}
	mult := payoutMultiplier(p.Rep)
	p.Gold += int(float64(baseGold) * mult)
	p.Rep = clampInt(p.Rep+repGain, -100, 100)
}

func applyFailurePenaltyLocked(store *Store, p *Player) {
	if p == nil {
		return
	}
	p.Rep = clampInt(p.Rep-10, -100, 100)
}

func finalizeDeliveredContractLocked(store *Store, p *Player, c *Contract, now time.Time) {
	if c == nil || p == nil || c.Status == "Completed" {
		return
	}
	outcome := computeDeliverOutcomeLocked(store, p, c)

	c.Status = "Completed"
	p.Gold += outcome.RewardGold
	p.Rep = clampInt(p.Rep+outcome.RepDelta, -100, 100)
	p.Heat = maxInt(0, p.Heat+outcome.HeatDelta)
	if p.Rumors > 0 {
		p.Rumors--
	}
	incrementCompletedCountersLocked(p, now)
	addEventLocked(store, Event{Type: "Consequence", Severity: 1, Text: stanceEventText(outcome.Stance), At: now})
}

func incrementCompletedCountersLocked(p *Player, now time.Time) {
	if p == nil {
		return
	}
	ensureTodayCounterLocked(p, now)
	p.CompletedContracts++
	p.CompletedContractsToday++
}

func ensureTodayCounterLocked(p *Player, now time.Time) {
	if p == nil {
		return
	}
	today := now.UTC().Format("2006-01-02")
	if p.CompletedTodayDateUTC != today {
		p.CompletedTodayDateUTC = today
		p.CompletedContractsToday = 0
	}
}

func adjustHeatForDeliveryLocked(p *Player, contractType string) {
	if p == nil {
		return
	}
	if contractType == "Smuggling" {
		p.Heat++
	}
}

func ensurePlayerLocked(store *Store, w http.ResponseWriter, r *http.Request) *Player {
	var pid string
	if c, err := r.Cookie(cookieName); err == nil && c.Value != "" {
		pid = c.Value
	} else {
		pid = generateID()
		http.SetCookie(w, &http.Cookie{
			Name:     cookieName,
			Value:    pid,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})
	}

	p := store.Players[pid]
	if p == nil {
		p = &Player{ID: pid, Name: uniqueGuestNameLocked(store), Gold: initialPlayerGold, Rep: 0, LastSeen: time.Now().UTC()}
		store.Players[pid] = p
		setToastLocked(store, pid, fmt.Sprintf("You arrive as %s.", p.Name))
		addEventLocked(store, Event{Type: "Join", Severity: 1, Text: fmt.Sprintf("[%s] enters the city under a borrowed name.", p.Name), At: time.Now().UTC()})
	}
	return p
}

func uniqueGuestNameLocked(store *Store) string {
	base := fmt.Sprintf("%s %s (Guest)", randomFrom(store.rng, nameFirst), randomFrom(store.rng, nameLast))
	if !playerNameExistsLocked(store, base) {
		return base
	}
	candidate := fmt.Sprintf("%s #%s", base, randomSuffix())
	if !playerNameExistsLocked(store, candidate) {
		return candidate
	}
	for {
		candidate := fmt.Sprintf("%s %s (Guest) #%s", randomFrom(store.rng, nameFirst), randomFrom(store.rng, nameLast), randomSuffix())
		if !playerNameExistsLocked(store, candidate) {
			return candidate
		}
	}
}

func playerNameExistsLocked(store *Store, name string) bool {
	for _, p := range store.Players {
		if p.Name == name {
			return true
		}
	}
	return false
}

func randomSuffix() string {
	buf := make([]byte, 2)
	if _, err := io.ReadFull(rand.Reader, buf); err != nil {
		return "X00"
	}
	enc := strings.ToUpper(base64.RawURLEncoding.EncodeToString(buf))
	if len(enc) >= 3 {
		return enc[:3]
	}
	return "X00"
}

func generateID() string {
	buf := make([]byte, 18)
	if _, err := io.ReadFull(rand.Reader, buf); err != nil {
		return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}

func randomFrom(r *mathrand.Rand, values []string) string {
	return values[r.Intn(len(values))]
}

func addEventLocked(store *Store, e Event) {
	store.NextEventID++
	e.ID = store.NextEventID
	e.DayNumber = store.World.DayNumber
	e.Subphase = store.World.Subphase
	if e.At.IsZero() {
		e.At = time.Now().UTC()
	}
	store.Events = append(store.Events, e)
	if len(store.Events) > maxEvents {
		store.Events = store.Events[len(store.Events)-maxEvents:]
	}
}

func addChatLocked(store *Store, msg ChatMessage) {
	store.NextChatID++
	msg.ID = store.NextChatID
	if msg.At.IsZero() {
		msg.At = time.Now().UTC()
	}
	store.Chat = append(store.Chat, msg)
	if len(store.Chat) > maxChat {
		store.Chat = store.Chat[len(store.Chat)-maxChat:]
	}
}

func issueContractLocked(store *Store, ctype string, deadline int) {
	store.NextContractID++
	id := fmt.Sprintf("c-%d", store.NextContractID)
	store.Contracts[id] = &Contract{ID: id, Type: ctype, DeadlineTicks: deadline, Status: "Issued", IssuedAtTick: store.TickCount}
}

func hasActiveContractLocked(store *Store, ctype string) bool {
	for _, c := range store.Contracts {
		if c.Type == ctype && (c.Status == "Issued" || c.Status == "Accepted") {
			return true
		}
	}
	return false
}

func playerAcceptedCountLocked(store *Store, playerID string) int {
	n := 0
	for _, c := range store.Contracts {
		if c.Status == "Accepted" && c.OwnerPlayerID == playerID {
			n++
		}
	}
	return n
}

func sortedContractsLocked(store *Store) []*Contract {
	out := make([]*Contract, 0, len(store.Contracts))
	for _, c := range store.Contracts {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Status != out[j].Status {
			return out[i].Status < out[j].Status
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func onlinePlayersLocked(store *Store, now time.Time) []*Player {
	var out []*Player
	for _, p := range store.Players {
		if now.Sub(p.LastSeen) <= onlineWindow {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func setToastLocked(store *Store, pid, text string) {
	store.ToastByPlayer[pid] = text
}

func popToastLocked(store *Store, pid string) string {
	msg := store.ToastByPlayer[pid]
	delete(store.ToastByPlayer, pid)
	return msg
}

func peekToastLocked(store *Store, pid string) string {
	return store.ToastByPlayer[pid]
}

func buildPageDataLocked(store *Store, playerID string, consumeToast bool) PageData {
	now := time.Now().UTC()
	p := store.Players[playerID]
	if p == nil {
		return PageData{}
	}
	ensureTodayCounterLocked(p, now)
	today := now.UTC().Format("2006-01-02")

	contractView := func(c *Contract) ContractView {
		urgency := ""
		if c.Status == "Issued" || c.Status == "Accepted" {
			if c.DeadlineTicks <= 1 {
				urgency = "urgent"
			} else if c.DeadlineTicks <= 2 {
				urgency = "warning"
			}
		}
		owner := c.OwnerName
		if owner == "" {
			owner = "-"
		} else if ownerP := store.Players[c.OwnerPlayerID]; ownerP != nil {
			owner = fmt.Sprintf("%s (%s)", ownerP.Name, reputationTitle(ownerP.Rep))
		}
		canAccept := c.Status == "Issued"
		canIgnore := c.Status == "Issued"
		canAbandon := c.Status == "Accepted" && c.OwnerPlayerID == p.ID
		canDeliver := (c.Status == "Accepted" && c.OwnerPlayerID == p.ID) || (c.Status == "Fulfilled" && c.OwnerPlayerID == p.ID)

		showOutcome := c.OwnerPlayerID == p.ID && (c.Status == "Accepted" || c.Status == "Fulfilled")
		deliverDisabled := false
		outcomeLabel := ""
		outcomeNote := ""
		var outcome DeliverOutcome
		if showOutcome {
			outcome = computeDeliverOutcomeLocked(store, p, c)
			outcomeLabel = fmt.Sprintf("%+dg, %+d rep, %+d heat", outcome.RewardGold, outcome.RepDelta, outcome.HeatDelta)
			if c.Status == "Accepted" {
				outcomeNote = "Costs 2g to attempt."
				if p.Gold < 2 {
					deliverDisabled = true
					outcomeNote = "Need 2g to attempt."
				}
				if lastDeliverAt, ok := store.LastDeliverAt[p.ID]; ok {
					cooldownRemaining := deliverCooldown - now.Sub(lastDeliverAt)
					if cooldownRemaining > 0 {
						remainingSeconds := int(cooldownRemaining.Seconds())
						if remainingSeconds == 0 {
							remainingSeconds = 1
						}
						deliverDisabled = true
						if outcomeNote != "" {
							outcomeNote += " "
						}
						outcomeNote += fmt.Sprintf("Delivery cooldown: %ds.", remainingSeconds)
					}
				}
			}
			if p.Rumors > 0 {
				if outcomeNote != "" {
					outcomeNote += " "
				}
				outcomeNote += "Rumor bonus ready."
			}
		}

		deliverLabel := "Deliver"
		if canDeliver && showOutcome {
			netGold := outcome.RewardGold
			if c.Status == "Accepted" && c.OwnerPlayerID == p.ID {
				netGold -= 2
			}
			deliverLabel = fmt.Sprintf("Deliver (%+dg)", netGold)
		}
		return ContractView{
			ID:            c.ID,
			Type:          c.Type,
			Status:        c.Status,
			DeadlineTicks: c.DeadlineTicks,
			OwnerName:     owner,
			Stance:        normalizeContractStance(c.Stance),
			UrgencyClass:  urgency,
			CanAccept:     canAccept,
			CanIgnore:     canIgnore,
			CanAbandon:    canAbandon,
			CanDeliver:    canDeliver,
			DeliverLabel:  deliverLabel,
			DeliverDisabled: deliverDisabled,
			ShowOutcome:   showOutcome,
			OutcomeLabel:  outcomeLabel,
			OutcomeNote:   outcomeNote,
		}
	}

	type scoredContractView struct {
		View       ContractView
		Group      int
		Deadline   int
		IssuedAt   int64
		SequenceID int
		ID         string
	}

	scored := []scoredContractView{}
	totalContractN := 0
	for _, c := range sortedContractsLocked(store) {
		if p.Rep < -50 && c.Type == "Smuggling" && c.Status == "Issued" {
			continue
		}
		totalContractN++
		cv := contractView(c)
		group := 6
		switch c.Status {
		case "Accepted":
			if c.OwnerPlayerID == p.ID {
				group = 0
			} else {
				group = 3
			}
		case "Fulfilled":
			if c.OwnerPlayerID == p.ID {
				group = 1
			} else {
				group = 5
			}
		case "Issued":
			group = 2
		case "Ignored":
			group = 4
		}
		scored = append(scored, scoredContractView{
			View:       cv,
			Group:      group,
			Deadline:   c.DeadlineTicks,
			IssuedAt:   c.IssuedAtTick,
			SequenceID: parseContractSequence(c.ID),
			ID:         c.ID,
		})
	}

	sort.Slice(scored, func(i, j int) bool {
		a := scored[i]
		b := scored[j]
		if a.Group != b.Group {
			return a.Group < b.Group
		}
		// Keep urgent/actionable contracts first for active groups.
		if a.Group <= 5 {
			if a.Deadline != b.Deadline {
				return a.Deadline < b.Deadline
			}
			if a.IssuedAt != b.IssuedAt {
				return a.IssuedAt > b.IssuedAt
			}
			if a.SequenceID != b.SequenceID {
				return a.SequenceID > b.SequenceID
			}
			return a.ID < b.ID
		}
		// For terminal contracts, show most recent first.
		if a.IssuedAt != b.IssuedAt {
			return a.IssuedAt > b.IssuedAt
		}
		if a.SequenceID != b.SequenceID {
			return a.SequenceID > b.SequenceID
		}
		return a.ID > b.ID
	})

	if len(scored) > maxVisibleContracts {
		scored = scored[:maxVisibleContracts]
	}
	contracts := make([]ContractView, 0, len(scored))
	for _, s := range scored {
		contracts = append(contracts, s.View)
	}

	events := make([]EventView, 0, len(store.Events))
	for _, e := range store.Events {
		events = append(events, EventView{DayNumber: e.DayNumber, Subphase: e.Subphase, Text: e.Text, At: e.At.Format("15:04:05")})
	}
	if len(events) > 3 {
		events = events[len(events)-3:]
	}

	players := make([]PlayerSummary, 0, len(store.Players))
	for _, pl := range store.Players {
		players = append(players, PlayerSummary{Name: pl.Name, Rep: pl.Rep, Title: reputationTitle(pl.Rep), Gold: pl.Gold, Online: now.Sub(pl.LastSeen) <= onlineWindow})
	}
	sort.Slice(players, func(i, j int) bool { return players[i].Name < players[j].Name })

	chat := []ChatView{}
	for _, m := range store.Chat {
		if !messageVisibleToPlayer(m, p.ID) {
			continue
		}
		fromTitle := ""
		if from := store.Players[m.FromPlayerID]; from != nil {
			fromTitle = reputationTitle(from.Rep)
		}
		chat = append(chat, ChatView{FromName: m.FromName, FromTitle: fromTitle, ToName: m.ToName, Text: m.Text, Kind: m.Kind, At: m.At.Format("15:04:05")})
	}
	if len(chat) > 80 {
		chat = chat[len(chat)-80:]
	}

	seatOrder := []string{"harbor_master", "master_of_coin", "high_curate"}
	seats := make([]SeatView, 0, len(seatOrder))
	for _, seatID := range seatOrder {
		seat := store.Seats[seatID]
		if seat == nil {
			continue
		}
		instName := ""
		if inst := store.Institutions[seat.InstitutionID]; inst != nil {
			instName = inst.Name
		}
		seats = append(seats, SeatView{
			ID:                  seat.ID,
			Name:                seat.Name,
			InstitutionName:     instName,
			HolderName:          seat.HolderName,
			TenureTicksLeft:     seat.TenureTicksLeft,
			ElectionWindowTicks: seat.ElectionWindowTicks,
			IsElectionOpen:      seat.ElectionWindowTicks > 0,
			CanCampaign:         seat.ElectionWindowTicks > 0,
			CanChallenge:        seat.ElectionWindowTicks == 0 && seat.HolderPlayerID != p.ID,
			CanToggleTaxHigh:    seat.ID == "master_of_coin" && seat.HolderPlayerID == p.ID && store.Policies.TaxRatePct != 20,
			CanToggleTaxLow:     seat.ID == "master_of_coin" && seat.HolderPlayerID == p.ID && store.Policies.TaxRatePct != 5,
			CanTogglePermit:     seat.ID == "harbor_master" && seat.HolderPlayerID == p.ID,
			CanToggleEmbargo:    seat.ID == "harbor_master" && seat.HolderPlayerID == p.ID,
		})
	}

	playerOptions := make([]PlayerOption, 0, len(store.Players))
	for _, pp := range store.Players {
		if pp.ID == p.ID {
			continue
		}
		playerOptions = append(playerOptions, PlayerOption{ID: pp.ID, Name: pp.Name})
	}
	sort.Slice(playerOptions, func(i, j int) bool { return playerOptions[i].Name < playerOptions[j].Name })

	rumors := make([]RumorView, 0, len(store.Rumors))
	for _, r := range store.Rumors {
		rumors = append(rumors, RumorView{
			ID:          r.ID,
			Claim:       r.Claim,
			Topic:       r.Topic,
			TargetName:  r.TargetName,
			SourceName:  r.SourceName,
			Credibility: r.Credibility,
			Spread:      r.Spread,
			Decay:       r.Decay,
		})
	}
	sort.Slice(rumors, func(i, j int) bool { return rumors[i].ID > rumors[j].ID })
	if len(rumors) > 8 {
		rumors = rumors[:8]
	}

	evidence := make([]EvidenceView, 0, len(store.Evidence))
	for _, ev := range store.Evidence {
		if ev.SourcePlayerID != p.ID {
			continue
		}
		evidence = append(evidence, EvidenceView{
			ID:         ev.ID,
			Topic:      ev.Topic,
			TargetName: ev.TargetName,
			SourceName: ev.SourceName,
			Strength:   ev.Strength,
			ExpiryIn:   int64(maxInt(0, int(ev.ExpiryTick-store.TickCount))),
		})
	}
	sort.Slice(evidence, func(i, j int) bool { return evidence[i].ID > evidence[j].ID })
	if len(evidence) > 8 {
		evidence = evidence[:8]
	}

	loans := make([]LoanView, 0, len(store.Loans))
	for _, ln := range store.Loans {
		if ln.BorrowerPlayerID != p.ID && ln.LenderPlayerID != p.ID {
			continue
		}
		loans = append(loans, LoanView{
			ID:           ln.ID,
			LenderName:   ln.LenderName,
			BorrowerName: ln.BorrowerName,
			Remaining:    ln.Remaining,
			DueIn:        int64(maxInt(0, int(ln.DueTick-store.TickCount))),
			Status:       ln.Status,
		})
	}
	sort.Slice(loans, func(i, j int) bool { return loans[i].ID > loans[j].ID })

	obligations := make([]ObligationView, 0, len(store.Obligations))
	for _, ob := range store.Obligations {
		if ob.DebtorPlayerID != p.ID && ob.CreditorPlayerID != p.ID {
			continue
		}
		obligations = append(obligations, ObligationView{
			ID:           ob.ID,
			CreditorName: ob.CreditorName,
			DebtorName:   ob.DebtorName,
			Reason:       ob.Reason,
			Severity:     ob.Severity,
			DueIn:        int64(maxInt(0, int(ob.DueTick-store.TickCount))),
			Status:       ob.Status,
		})
	}
	sort.Slice(obligations, func(i, j int) bool { return obligations[i].ID > obligations[j].ID })

	toast := ""
	if consumeToast {
		toast = popToastLocked(store, playerID)
	} else {
		toast = peekToastLocked(store, playerID)
	}

	remaining := store.TickEvery - now.Sub(store.LastTickAt)
	if remaining < 0 {
		remaining = 0
	}
	tickStatus := fmt.Sprintf("Next tick in %ds · cadence %ds", int(remaining.Seconds()), int(store.TickEvery.Seconds()))

	highImpactRemaining := highImpactDailyCap
	if store.DailyActionDate[playerID] == today {
		highImpactRemaining = highImpactDailyCap - store.DailyHighImpactN[playerID]
	}
	if highImpactRemaining < 0 {
		highImpactRemaining = 0
	}

	investigateCooldown := 0
	if lastTick, ok := store.LastInvestigateAt[p.ID]; ok {
		diff := int(store.TickCount - lastTick)
		if diff < 3 {
			investigateCooldown = 3 - diff
		}
	}
	investigateDisabled := investigateCooldown > 0
	investigateLabel := "Investigate"
	if investigateDisabled {
		investigateLabel = fmt.Sprintf("Investigate (%dt)", investigateCooldown)
	}

	return PageData{
		NowUTC:      now.Format(time.RFC3339),
		Player:      p,
		PlayerTitle: reputationTitle(p.Rep),
		Standing: StandingView{
			ReputationValue: p.Rep,
			ReputationLabel: standingReputationLabel(p.Rep),
			HeatValue:       p.Heat,
			HeatLabel:       standingHeatLabel(p.Heat),
			WealthGold:      p.Gold,
			CompletedToday:  p.CompletedContractsToday,
			CompletedTotal:  p.CompletedContracts,
			Rumors:          p.Rumors,
		},
		World:            store.World,
		Situation:        store.World.Situation,
		HighImpactRemaining: highImpactRemaining,
		HighImpactCap:       highImpactDailyCap,
		InvestigateDisabled: investigateDisabled,
		InvestigateLabel:    investigateLabel,
		Contracts:        contracts,
		Events:           events,
		Players:          players,
		Chat:             chat,
		Toast:            toast,
		AcceptedCount:    playerAcceptedCountLocked(store, playerID),
		VisibleContractN: len(contracts),
		TotalContractN:   totalContractN,
		Seats:            seats,
		Policies:         store.Policies,
		Rumors:           rumors,
		Evidence:         evidence,
		Loans:            loans,
		Obligations:      obligations,
		PlayerOptions:    playerOptions,
		TickStatus:       tickStatus,
	}
}

func parseContractSequence(id string) int {
	if strings.HasPrefix(id, "c-") {
		n, err := strconv.Atoi(strings.TrimPrefix(id, "c-"))
		if err == nil {
			return n
		}
	}
	return -1
}

func messageVisibleToPlayer(m ChatMessage, playerID string) bool {
	if m.Kind == "global" {
		return true
	}
	if m.ToPlayerID == "" {
		return true
	}
	return m.ToPlayerID == playerID || m.FromPlayerID == playerID
}

func renderPage(w http.ResponseWriter, tmpl *template.Template, name string, data PageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// htmx response strategy: /action and /frag/dashboard return dashboard HTML as the primary swap
// target and include out-of-band (OOB) fragments so event log, players list, and toast stay in sync.
func renderActionLikeResponse(w http.ResponseWriter, tmpl *template.Template, data PageData, includeChatOOB bool) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "dashboard", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = tmpl.ExecuteTemplate(w, "header_oob", data)
	_ = tmpl.ExecuteTemplate(w, "events_oob", data)
	_ = tmpl.ExecuteTemplate(w, "players_oob", data)
	_ = tmpl.ExecuteTemplate(w, "institutions_oob", data)
	_ = tmpl.ExecuteTemplate(w, "intel_oob", data)
	_ = tmpl.ExecuteTemplate(w, "ledger_oob", data)
	if includeChatOOB {
		_ = tmpl.ExecuteTemplate(w, "chat_oob", data)
	}
	_ = tmpl.ExecuteTemplate(w, "toast_oob", data)
}

func renderChatResponse(w http.ResponseWriter, tmpl *template.Template, data PageData, includeOOB bool) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "chat_inner", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if includeOOB {
		_ = tmpl.ExecuteTemplate(w, "header_oob", data)
		_ = tmpl.ExecuteTemplate(w, "events_oob", data)
		_ = tmpl.ExecuteTemplate(w, "players_oob", data)
		_ = tmpl.ExecuteTemplate(w, "institutions_oob", data)
		_ = tmpl.ExecuteTemplate(w, "intel_oob", data)
		_ = tmpl.ExecuteTemplate(w, "ledger_oob", data)
		_ = tmpl.ExecuteTemplate(w, "toast_oob", data)
	}
}

func isAdmin(r *http.Request) bool {
	if r.URL.Query().Get("token") == adminToken {
		return true
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return host == "localhost" || (ip != nil && ip.IsLoopback())
}

func adminTokenSuffix(r *http.Request) string {
	if r.URL.Query().Get("token") == adminToken {
		return "?token=" + adminToken
	}
	return ""
}

func grainTierFromSupply(v int) string {
	switch {
	case v > 200:
		return "Stable"
	case v >= 101:
		return "Tight"
	case v >= 41:
		return "Scarce"
	default:
		return "Critical"
	}
}

func unrestTierFromValue(v int) string {
	switch {
	case v <= 10:
		return "Calm"
	case v <= 30:
		return "Uneasy"
	case v <= 60:
		return "Unstable"
	default:
		return "Rioting"
	}
}

func grainTierNarrative(from, to string) string {
	if from == to {
		return ""
	}
	if to == "Tight" {
		return "Grain supply tightens."
	}
	if to == "Scarce" {
		return "Grain stores thin across the city."
	}
	if to == "Critical" {
		return "Grain stores fall below emergency reserves."
	}
	return "Fresh grain reaches the markets, easing shortages."
}

func unrestTierNarrative(from, to string) string {
	if from == to {
		return ""
	}
	if to == "Uneasy" {
		return "Whispers of worry spread through the streets."
	}
	if to == "Unstable" {
		return "Tension rises as crowds gather and tempers flare."
	}
	if to == "Rioting" {
		return "The city erupts into open unrest."
	}
	return "The streets quiet as tensions ease."
}

func deriveSituation(grainTier, unrestTier string) string {
	switch {
	case grainTier == "Stable" && unrestTier == "Calm":
		return "The city breathes-uneasy peace holds."
	case grainTier == "Scarce" && unrestTier == "Uneasy":
		return "Shortages spread quiet panic through the markets."
	case grainTier == "Critical" && unrestTier == "Unstable":
		return "Hunger sharpens into anger; deals turn desperate."
	case grainTier == "Critical" && unrestTier == "Rioting":
		return "The streets burn with desperation and blame."
	case unrestTier == "Rioting":
		return "Fires, fear, and blame race faster than grain."
	case grainTier == "Critical":
		return "Emergency stores fray as every convoy is contested."
	case grainTier == "Scarce":
		return "Every sack matters and every alley has a price."
	case unrestTier == "Unstable":
		return "Crowds watch each cart as trust thins by the hour."
	default:
		return "Merchants bargain in low voices while the city waits."
	}
}

func reputationTitle(rep int) string {
	switch {
	case rep >= 50:
		return "Renowned"
	case rep >= 20:
		return "Trusted"
	case rep >= -19:
		return "Unknown"
	case rep >= -49:
		return "Shady"
	default:
		return "Notorious"
	}
}

func standingReputationLabel(rep int) string {
	switch {
	case rep <= -20:
		return "Pariah"
	case rep <= -6:
		return "Disliked"
	case rep <= 5:
		return "Unknown"
	case rep <= 19:
		return "Trusted"
	default:
		return "Esteemed"
	}
}

func standingHeatLabel(heat int) string {
	switch {
	case heat <= 0:
		return "Clean"
	case heat <= 4:
		return "Watched"
	default:
		return "Wanted"
	}
}

func payoutMultiplier(rep int) float64 {
	v := 1.0 + float64(rep)/200.0
	if v < 0.75 {
		return 0.75
	}
	if v > 1.5 {
		return 1.5
	}
	return v
}

func normalizeContractStance(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "fast":
		return contractStanceFast
	case "quiet":
		return contractStanceQuiet
	default:
		return contractStanceCareful
	}
}

func baseContractRewardGold(p *Player, c *Contract) int {
	baseGold := 25
	if c != nil && c.Type == "Smuggling" {
		baseGold = 35
	}
	rep := 0
	if p != nil {
		rep = p.Rep
	}
	return int(float64(baseGold) * payoutMultiplier(rep))
}

func baseContractRepDelta(c *Contract) int {
	if c != nil && c.Type == "Smuggling" {
		return 3
	}
	return 8
}

func baseContractHeatDelta(c *Contract) int {
	if c != nil && c.Type == "Smuggling" {
		return 1
	}
	return 0
}

func computeDeliverOutcomeLocked(store *Store, p *Player, c *Contract) DeliverOutcome {
	stance := contractStanceCareful
	if c != nil {
		stance = normalizeContractStance(c.Stance)
	}
	reward := baseContractRewardGold(p, c)
	repDelta := baseContractRepDelta(c)
	heatDelta := baseContractHeatDelta(c)

	switch stance {
	case contractStanceCareful:
		reward = int(float64(reward) * 0.9)
		heatDelta -= 1
		repDelta += 1
	case contractStanceFast:
		reward = int(float64(reward) * 1.1)
		heatDelta += 2
	case contractStanceQuiet:
		reward = int(float64(reward) * 0.8)
		heatDelta -= 2
	}
	if c != nil && c.Type == "Smuggling" {
		heatDelta++
	}
	if p != nil && p.Rumors > 0 {
		reward += rumorDeliverBonusGold
	}
	if store != nil {
		reward = reward * (100 - clampInt(store.Policies.TaxRatePct, 0, 40)) / 100
		if c != nil && c.Type == "Smuggling" && store.Policies.SmugglingEmbargoTicks > 0 {
			reward += 6
		}
	}
	return DeliverOutcome{
		RewardGold: reward,
		HeatDelta:  heatDelta,
		RepDelta:   repDelta,
		Stance:     stance,
	}
}

func stanceEventText(stance string) string {
	switch normalizeContractStance(stance) {
	case contractStanceFast:
		return "You move fast; whispers follow your wake."
	case contractStanceQuiet:
		return "You keep it quiet; fewer eyes notice."
	default:
		return "You deliver carefully; the Watch seems less interested."
	}
}

func fulfillChanceForTier(tier string) int {
	switch tier {
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

func deliverChanceByTier(tier string) int {
	switch tier {
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

func resolveWhisperTargetLocked(store *Store, tail string) (*Player, string) {
	tail = strings.TrimSpace(tail)
	if tail == "" {
		return nil, ""
	}
	players := make([]*Player, 0, len(store.Players))
	for _, p := range store.Players {
		players = append(players, p)
	}
	sort.Slice(players, func(i, j int) bool {
		return len(normalizeWhisperName(players[i].Name)) > len(normalizeWhisperName(players[j].Name))
	})
	tailParts := strings.Fields(strings.ToLower(strings.TrimSpace(tail)))
	for _, p := range players {
		nameParts := strings.Fields(normalizeWhisperName(p.Name))
		if len(tailParts) < len(nameParts)+1 {
			continue
		}
		matches := true
		for i := range nameParts {
			if tailParts[i] != nameParts[i] {
				matches = false
				break
			}
		}
		if !matches {
			continue
		}
		offset := len(nameParts)
		if len(tailParts) > offset && tailParts[offset] == "(guest)" {
			offset++
		}
		body := strings.TrimSpace(strings.Join(tailParts[offset:], " "))
		return p, body
	}
	parts := strings.Fields(tail)
	if len(parts) < 2 {
		return nil, ""
	}
	target := findPlayerByNameTokenLocked(store, parts[0])
	if target == nil {
		return nil, strings.TrimSpace(strings.Join(parts[1:], " "))
	}
	return target, strings.TrimSpace(strings.Join(parts[1:], " "))
}

func findPlayerByNameTokenLocked(store *Store, token string) *Player {
	needle := normalizeWhisperName(token)
	for _, p := range store.Players {
		if normalizeWhisperName(p.Name) == needle || strings.HasPrefix(normalizeWhisperName(p.Name), needle+" ") {
			return p
		}
	}
	return nil
}

func normalizeWhisperName(v string) string {
	v = strings.TrimSpace(strings.ToLower(v))
	v = strings.TrimSuffix(v, " (guest)")
	return strings.TrimSpace(v)
}

func rollPercent(r *mathrand.Rand, chance int) bool {
	if chance <= 0 {
		return false
	}
	if chance >= 100 {
		return true
	}
	return r.Intn(100) < chance
}

func tooSoon(last time.Time, now time.Time, d time.Duration) bool {
	if last.IsZero() {
		return false
	}
	return now.Sub(last) < d
}

func clampInt(v, low, high int) int {
	if v < low {
		return low
	}
	if v > high {
		return high
	}
	return v
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
