package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"math"
	mathrand "math/rand"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/joho/godotenv"
)

const (
	cookieName                  = "pid"
	adminAuthCookieName         = "admin_auth"
	adminCSRFCookieName         = "admin_csrf"
	adminAuthHeaderName         = "X-Admin-Token"
	adminCSRFHeaderName         = "X-CSRF-Token"
	playerCookieSecretEnvName   = "PLAYER_COOKIE_SECRET"
	playerCookieSecureEnvName   = "PLAYER_COOKIE_SECURE"
	adminTokenEnvName           = "ADMIN_TOKEN"
	adminLoopbackEnvName        = "ALLOW_LOOPBACK_ADMIN"
	adminCookieSecureEnvName    = "ADMIN_COOKIE_SECURE"
	maxEvents                   = 300
	maxChat                     = 200
	maxDiplomacyMessages        = 200
	maxVisibleIntercepts        = 6
	maxVisibleContracts         = 12
	maxVisibleMessages          = 20
	onlineWindow                = 60 * time.Second
	inactiveWindow              = 120 * time.Second
	actionCooldown              = 2 * time.Second
	deliverCooldown             = 10 * time.Second
	chatCooldown                = 2 * time.Second
	messageCooldown             = 5 * time.Second
	serverAddr                  = ":8080"
	templateRoot                = "templates"
	initialPlayerGold           = 20
	rumorInvestigateGain        = 1
	rumorWhisperGain            = 1
	rumorDeliverBonusGold       = 3
	seatTenureTicks             = 8
	electionWindowTicks         = 2
	highImpactDailyCap          = 3
	loanDueTicks                = 4
	grainUnitPerSack            = 6
	marketMaxTrade              = 12
	reliefSackCost              = 3
	wantedHeatThreshold         = 10
	bountyEvidenceMin           = 4
	bountyDeadlineTicks         = 4
	supplyContractMinSacks      = 2
	supplyContractMaxSacks      = 10
	supplyContractMinReward     = 6
	supplyContractMaxReward     = 60
	supplyContractDeadlineTicks = 3
	obligationDueTicks          = 3
	projectMaxActive            = 4
	messageSubjectMax           = 80
	messageBodyMax              = 260
	fieldworkCooldownTicks      = 2
	fieldworkSupplyCost         = 1
	permitDurationTicks         = 3
	relicAppraiseCost           = 4
	relicMaxVisible             = 6
	scryReportDurationTicks     = 3
	interceptDurationTicks      = 3
	forgeEvidenceCost           = 3
	forgeEvidenceDurationTicks  = 3
	bribeAccessBaseTicks        = 2
	bribeAccessMaxTicks         = 5
	warrantDurationTicks        = 3
	warrantHeatDelta            = 3
	warrantRewardBonus          = 10
	sealedMessageCost           = 2
	sealedInterceptPenalty      = 15
	locationCapital             = "capital"
	locationHarbor              = "harbor"
	locationFrontier            = "frontier"
	locationRuins               = "ruins"
	inquestRumorCredibilityDrop = 2
	inquestRumorSpreadDrop      = 3
	inquestRumorDecayDrop       = 2
	adminResetConfirmPhrase     = "RESET WORLD"
	adminMaxManualTicks         = 24
	maxFormBodyBytes            = 1 << 20
)

var (
	playerCookieSecretOnce sync.Once
	playerCookieSecret     []byte
)

type WorldState struct {
	DayNumber                    int
	Subphase                     string
	GrainSupply                  int
	GrainTier                    string
	UnrestValue                  int
	UnrestTier                   string
	RestrictedMarketsTicks       int
	WardNetworkTicks             int
	CriticalTickStreak           int
	CriticalStreakPenaltyApplied bool
	Situation                    string
}

type Player struct {
	ID                      string
	Name                    string
	Gold                    int
	Grain                   int
	Rep                     int
	Heat                    int
	Rumors                  int
	CompletedContracts      int
	CompletedContractsToday int
	CompletedTodayDateUTC   string
	RiteImmunityTicks       int
	BribeAccessTicks        int
	LocationID              string
	TravelToID              string
	TravelTicksLeft         int
	TravelTotalTicks        int
	LastSeen                time.Time
	SoftDeletedAt           time.Time
	HardDeletedAt           time.Time
}

type Contract struct {
	ID             string
	Type           string
	DeadlineTicks  int
	Status         string
	OwnerPlayerID  string
	OwnerName      string
	IssuerPlayerID string
	IssuerName     string
	Stance         string
	IssuedAtTick   int64
	TargetPlayerID string
	TargetName     string
	BountyReward   int
	BountyEvidence int
	RewardGold     int
	SupplySacks    int
	Warranted      bool
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

type DiplomaticMessage struct {
	ID           int64
	FromPlayerID string
	FromName     string
	ToPlayerID   string
	ToName       string
	Subject      string
	Body         string
	At           time.Time
	Sealed       bool
}

type InterceptedMessage struct {
	ID            int64
	OwnerPlayerID string
	OwnerName     string
	FromName      string
	ToName        string
	Subject       string
	Body          string
	At            time.Time
	ExpiryTick    int64
	Sealed        bool
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
	Forged         bool
}

type ScryReport struct {
	ID              int64
	OwnerPlayerID   string
	TargetPlayerID  string
	TargetName      string
	LocationID      string
	TravelToID      string
	TravelTicksLeft int
	Rep             int
	Heat            int
	Gold            int
	Grain           int
	ExpiryTick      int64
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
	TerminalAt       time.Time
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
	TerminalAt       time.Time
}

type Permit struct {
	PlayerID     string
	PlayerName   string
	IssuerID     string
	IssuerName   string
	TicksLeft    int
	TotalTicks   int
	IssuedAtTick int64
}

type Warrant struct {
	PlayerID     string
	PlayerName   string
	IssuerID     string
	IssuerName   string
	TicksLeft    int
	TotalTicks   int
	IssuedAtTick int64
}

type Relic struct {
	ID              int64
	Name            string
	Effect          string
	Power           int
	OwnerPlayerID   string
	OwnerName       string
	Status          string
	FoundAtTick     int64
	AppraisedAtTick int64
}

type RelicDefinition struct {
	Name   string
	Effect string
	Power  int
}

type Project struct {
	ID            string
	Type          string
	Name          string
	OwnerPlayerID string
	OwnerName     string
	CostGold      int
	CostGrain     int
	TicksLeft     int
	TotalTicks    int
}

type ProjectDefinition struct {
	Type             string
	Name             string
	Description      string
	CostGold         int
	CostGrain        int
	DurationTicks    int
	GrainDelta       int
	UnrestDelta      int
	RepDelta         int
	HeatDelta        int
	WardNetworkTicks int
}

type Crisis struct {
	Type        string
	Name        string
	Description string
	Severity    int
	TicksLeft   int
	TotalTicks  int
	Mitigated   bool
}

type CrisisDefinition struct {
	Type               string
	Name               string
	Description        string
	DurationTicks      int
	BaseSeverity       int
	GoldCost           int
	GrainCost          int
	ResponseLabel      string
	TickUnrestDelta    int
	TickGrainDelta     int
	ResolveRepDelta    int
	ResolveUnrestDelta int
	FailureUnrestDelta int
	FailureGrainDelta  int
}

type LocationDef struct {
	ID          string
	Name        string
	Description string
}

type Store struct {
	mu   sync.Mutex
	repo *SQLRepository

	World        WorldState
	Players      map[string]*Player
	Contracts    map[string]*Contract
	Institutions map[string]*Institution
	Seats        map[string]*Seat
	Policies     PolicyState
	Rumors       map[int64]*Rumor
	Evidence     map[int64]*Evidence
	ScryReports  map[int64]*ScryReport
	Intercepts   map[int64]*InterceptedMessage
	Loans        map[string]*Loan
	Obligations  map[string]*Obligation
	Permits      map[string]*Permit
	Warrants     map[string]*Warrant
	Relics       map[int64]*Relic
	Projects     map[string]*Project
	ActiveCrisis *Crisis

	Events   []Event
	Chat     []ChatMessage
	Messages []DiplomaticMessage

	NextEventID      int64
	NextContractID   int64
	NextChatID       int64
	NextMessageID    int64
	NextRumorID      int64
	NextEvidenceID   int64
	NextScryID       int64
	NextInterceptID  int64
	NextLoanID       int64
	NextObligationID int64
	NextProjectID    int64
	NextRelicID      int64

	LastDailyTickDate string
	LastTickAt        time.Time
	TickEvery         time.Duration
	TickCount         int64

	LastChatAt        map[string]time.Time
	LastMessageAt     map[string]time.Time
	LastActionAt      map[string]time.Time
	LastDeliverAt     map[string]time.Time
	LastInvestigateAt map[string]int64
	LastSeatActionAt  map[string]int64
	LastIntelActionAt map[string]int64
	LastFieldworkAt   map[string]int64
	DailyActionDate   map[string]string
	DailyHighImpactN  map[string]int
	ToastByPlayer     map[string]string
	LastCleanupDate   string

	rng *mathrand.Rand
}

type AdminDiagnostics struct {
	TotalPlayers           int      `json:"total_players"`
	OnlinePlayers          int      `json:"online_players"`
	TravelingPlayers       int      `json:"traveling_players"`
	TotalGold              int      `json:"total_gold"`
	TotalGrain             int      `json:"total_grain"`
	AvgGoldPerPlayer       int      `json:"avg_gold_per_player"`
	AvgGrainPerPlayer      int      `json:"avg_grain_per_player"`
	AvgRep                 int      `json:"avg_rep"`
	AvgHeat                int      `json:"avg_heat"`
	RichestPlayer          string   `json:"richest_player"`
	RichestGold            int      `json:"richest_gold"`
	RichestSharePct        int      `json:"richest_share_pct"`
	HottestPlayer          string   `json:"hottest_player"`
	HottestHeat            int      `json:"hottest_heat"`
	HighestRepPlayer       string   `json:"highest_rep_player"`
	HighestRep             int      `json:"highest_rep"`
	LowestRepPlayer        string   `json:"lowest_rep_player"`
	LowestRep              int      `json:"lowest_rep"`
	ContractsIssued        int      `json:"contracts_issued"`
	ContractsAccepted      int      `json:"contracts_accepted"`
	ContractsFulfilled     int      `json:"contracts_fulfilled"`
	ContractsFailed        int      `json:"contracts_failed"`
	OverdueActiveContracts int      `json:"overdue_active_contracts"`
	ContractStateAnomalies int      `json:"contract_state_anomalies"`
	OverdueActiveLoans     int      `json:"overdue_active_loans"`
	OverdueOpenObligations int      `json:"overdue_open_obligations"`
	WorldPressureLevel     string   `json:"world_pressure_level"`
	AlertCount             int      `json:"alert_count"`
	Alerts                 []string `json:"alerts"`
}

type PlayerSummary struct {
	Name      string
	Rep       int
	Title     string
	Gold      int
	Heat      int
	HeatLabel string
	Warrant   string
	Online    bool
	IconPath  string
	IconTint  string
}

type ContractView struct {
	ID              string
	Type            string
	Status          string
	DeadlineTicks   int
	OwnerName       string
	IssuerName      string
	Stance          string
	TargetName      string
	UrgencyClass    string
	CanAccept       bool
	CanIgnore       bool
	CanAbandon      bool
	CanCancel       bool
	CanDeliver      bool
	DeliverLabel    string
	DeliverDisabled bool
	ShowOutcome     bool
	OutcomeLabel    string
	OutcomeNote     string
	RequirementNote string
	RewardNote      string
	IsBounty        bool
	IsSupply        bool
	IconPath        string
	IconTint        string
}

type StandingView struct {
	ReputationValue int
	ReputationLabel string
	HeatValue       int
	HeatLabel       string
	WealthGold      int
	GrainStockpile  int
	CompletedToday  int
	CompletedTotal  int
	Rumors          int
	PermitStatus    string
	AccessStatus    string
	WarrantStatus   string
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

type MessageView struct {
	FromName  string
	ToName    string
	Subject   string
	Body      string
	Direction string
	At        string
	Sealed    bool
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
	CanIssuePermit      bool
	CanConductInquest   bool
	CanIssueWarrant     bool
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
	SourceNote string
}

type ScryReportView struct {
	ID           int64
	TargetName   string
	LocationName string
	TravelNote   string
	Rep          int
	Heat         int
	Gold         int
	Grain        int
	ExpiryIn     int64
}

type InterceptView struct {
	ID       int64
	FromName string
	ToName   string
	Subject  string
	Body     string
	At       string
	ExpiryIn int64
	Sealed   bool
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
	ID             string
	CreditorName   string
	DebtorName     string
	Reason         string
	Severity       int
	DueIn          int64
	Status         string
	Cost           int
	CanSettle      bool
	SettleLabel    string
	SettleDisabled bool
	CanForgive     bool
}

type PermitView struct {
	PlayerName string
	IssuerName string
	TicksLeft  int
}

type WarrantView struct {
	PlayerName string
	IssuerName string
	TicksLeft  int
}

type RelicView struct {
	ID                     int64
	Name                   string
	Status                 string
	EffectLabel            string
	CanAppraise            bool
	AppraiseDisabled       bool
	AppraiseDisabledReason string
	CanInvoke              bool
	InvokeDisabled         bool
	InvokeDisabledReason   string
}

type ProjectView struct {
	ID         string
	Name       string
	OwnerName  string
	TicksLeft  int
	EffectNote string
}

type ProjectOption struct {
	Type           string
	Name           string
	Description    string
	CostGold       int
	CostGrain      int
	DurationTicks  int
	Disabled       bool
	DisabledReason string
}

type CrisisView struct {
	Name                   string
	Description            string
	Severity               int
	TicksLeft              int
	TotalTicks             int
	ResponseLabel          string
	ResponseCost           string
	ResponseDisabled       bool
	ResponseDisabledReason string
}

type PlayerOption struct {
	ID   string
	Name string
}

type LocationOption struct {
	ID          string
	Name        string
	TravelTicks int
	Disabled    bool
	Reason      string
	IconPath    string
	IconTint    string
}

type PageData struct {
	NowUTC                  string
	Player                  *Player
	PlayerTitle             string
	Standing                StandingView
	World                   WorldState
	Situation               string
	HighImpactRemaining     int
	HighImpactCap           int
	InvestigateDisabled     bool
	InvestigateLabel        string
	MarketBasePrice         int
	MarketBuyPrice          int
	MarketSellPrice         int
	MarketSupplySacks       int
	MarketControlsTicks     int
	MarketControlsActive    bool
	MarketStockpile         int
	MarketMaxBuy            int
	MarketMaxSell           int
	MarketBuyDisabled       bool
	MarketSellDisabled      bool
	ReliefCost              int
	ReliefDisabled          bool
	ReliefLabel             string
	HasOtherPlayers         bool
	Contracts               []ContractView
	Events                  []EventView
	Players                 []PlayerSummary
	Chat                    []ChatView
	ChatDraft               string
	Messages                []MessageView
	MessageDraftSubject     string
	MessageDraftBody        string
	MessageDraftTargetID    string
	MessageDraftSealed      bool
	SealMessageCost         int
	SealMessageDisabled     bool
	SealMessageDisabledNote string
	Toast                   string
	AcceptedCount           int
	VisibleContractN        int
	TotalContractN          int
	Seats                   []SeatView
	Policies                PolicyState
	Rumors                  []RumorView
	Evidence                []EvidenceView
	ScryReports             []ScryReportView
	Intercepts              []InterceptView
	ForgeEvidenceCost       int
	ForgeEvidenceDisabled   bool
	ForgeEvidenceReason     string
	Loans                   []LoanView
	Obligations             []ObligationView
	Permits                 []PermitView
	Warrants                []WarrantView
	Relics                  []RelicView
	RelicAppraiseCost       int
	Projects                []ProjectView
	ProjectOptions          []ProjectOption
	Crisis                  *CrisisView
	PlayerOptions           []PlayerOption
	LocationName            string
	LocationDescription     string
	LocationIconPath        string
	LocationIconTint        string
	Traveling               bool
	TravelDestination       string
	TravelTicksLeft         int
	TravelTotalTicks        int
	LocationOptions         []LocationOption
	FieldworkAvailable      bool
	FieldworkAction         string
	FieldworkLabel          string
	FieldworkDescription    string
	FieldworkDisabled       bool
	FieldworkDisabledReason string
	FieldworkSupplyCost     int
	TickStatus              string
}

const (
	contractStanceCareful = "Careful"
	contractStanceFast    = "Fast"
	contractStanceQuiet   = "Quiet"
)

const (
	relicStatusUnappraised = "Unappraised"
	relicStatusAppraised   = "Appraised"
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
	loadDotEnv()
	tmpl := parseTemplates()
	store, err := newConfiguredStore()
	if err != nil {
		log.Fatal(err)
	}
	startTickScheduler(store)
	startCleanupScheduler(store)
	mux := newMux(store, tmpl)

	log.Printf("listening on http://localhost%s", serverAddr)
	server := &http.Server{
		Addr:              serverAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	log.Fatal(server.ListenAndServe())
}

func loadDotEnv() {
	if _, err := os.Stat(".env"); err != nil {
		return
	}
	if err := godotenv.Load(); err != nil {
		log.Printf("warning: failed to load .env: %v", err)
	}
}

func newMux(store *Store, tmpl *template.Template) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir("assets"))))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Concurrency model: lock for full handler to keep all reads/writes consistent and race-free.
		store.mu.Lock()
		defer store.mu.Unlock()
		defer store.persistLocked()

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
		defer store.persistLocked()
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
		defer store.persistLocked()
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
		defer store.persistLocked()
		p := ensurePlayerLocked(store, w, r)
		p.LastSeen = time.Now().UTC()
		renderPage(w, tmpl, "chat_inner", buildPageDataLocked(store, p.ID, false))
	})

	mux.HandleFunc("/frag/diplomacy", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		store.mu.Lock()
		defer store.mu.Unlock()
		defer store.persistLocked()
		p := ensurePlayerLocked(store, w, r)
		p.LastSeen = time.Now().UTC()
		renderPage(w, tmpl, "diplomacy_inner", buildPageDataLocked(store, p.ID, false))
	})

	mux.HandleFunc("/frag/players", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		store.mu.Lock()
		defer store.mu.Unlock()
		defer store.persistLocked()
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
		defer store.persistLocked()
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
		defer store.persistLocked()
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
		defer store.persistLocked()
		p := ensurePlayerLocked(store, w, r)
		p.LastSeen = time.Now().UTC()
		renderPage(w, tmpl, "ledger_inner", buildPageDataLocked(store, p.ID, false))
	})

	mux.HandleFunc("/frag/market", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		store.mu.Lock()
		defer store.mu.Unlock()
		defer store.persistLocked()
		p := ensurePlayerLocked(store, w, r)
		p.LastSeen = time.Now().UTC()
		renderPage(w, tmpl, "market_inner", buildPageDataLocked(store, p.ID, false))
	})

	mux.HandleFunc("/action", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !parsePostFormLimited(w, r, maxFormBodyBytes) {
			return
		}

		store.mu.Lock()
		defer store.mu.Unlock()
		defer store.persistLocked()

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
			Action:       strings.TrimSpace(r.FormValue("action")),
			ContractID:   strings.TrimSpace(r.FormValue("contract_id")),
			Stance:       strings.TrimSpace(r.FormValue("stance")),
			TargetID:     strings.TrimSpace(r.FormValue("target_id")),
			RelicID:      strings.TrimSpace(r.FormValue("relic_id")),
			Claim:        strings.TrimSpace(r.FormValue("claim")),
			Topic:        strings.TrimSpace(r.FormValue("topic")),
			LoanID:       strings.TrimSpace(r.FormValue("loan_id")),
			ObligationID: strings.TrimSpace(r.FormValue("obligation_id")),
			ProjectType:  strings.TrimSpace(r.FormValue("project_type")),
			LocationID:   strings.TrimSpace(r.FormValue("location_id")),
		}
		if n, err := strconv.Atoi(strings.TrimSpace(r.FormValue("amount"))); err == nil {
			input.Amount = n
		}
		if n, err := strconv.Atoi(strings.TrimSpace(r.FormValue("sacks"))); err == nil {
			input.Sacks = n
		}
		if n, err := strconv.Atoi(strings.TrimSpace(r.FormValue("reward"))); err == nil {
			input.Reward = n
		}

		handleActionInputLocked(store, p, now, input)
		renderActionLikeResponse(w, tmpl, buildPageDataLocked(store, p.ID, true), false)
	})

	mux.HandleFunc("/chat", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !parsePostFormLimited(w, r, maxFormBodyBytes) {
			return
		}

		store.mu.Lock()
		defer store.mu.Unlock()
		defer store.persistLocked()

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

	mux.HandleFunc("/message", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !parsePostFormLimited(w, r, maxFormBodyBytes) {
			return
		}

		store.mu.Lock()
		defer store.mu.Unlock()
		defer store.persistLocked()

		p := ensurePlayerLocked(store, w, r)
		now := time.Now().UTC()
		p.LastSeen = now

		targetID := strings.TrimSpace(r.FormValue("target_id"))
		subject := strings.TrimSpace(r.FormValue("subject"))
		body := strings.TrimSpace(r.FormValue("body"))
		sealed := strings.TrimSpace(r.FormValue("sealed")) != ""

		data := buildPageDataLocked(store, p.ID, true)
		data.MessageDraftTargetID = targetID
		data.MessageDraftSubject = subject
		data.MessageDraftBody = body
		data.MessageDraftSealed = sealed

		if tooSoon(store.LastMessageAt[p.ID], now, messageCooldown) {
			setToastLocked(store, p.ID, "Couriers need more time to return.")
			renderActionLikeResponse(w, tmpl, data, false)
			return
		}

		if targetID == "" {
			setToastLocked(store, p.ID, "Choose a recipient.")
			renderActionLikeResponse(w, tmpl, data, false)
			return
		}
		target := store.Players[targetID]
		if target == nil || target.ID == p.ID {
			setToastLocked(store, p.ID, "That recipient is unavailable.")
			renderActionLikeResponse(w, tmpl, data, false)
			return
		}
		if subject == "" || body == "" {
			setToastLocked(store, p.ID, "Subject and message are required.")
			renderActionLikeResponse(w, tmpl, data, false)
			return
		}
		if len(subject) > messageSubjectMax {
			setToastLocked(store, p.ID, fmt.Sprintf("Subject too long (max %d).", messageSubjectMax))
			renderActionLikeResponse(w, tmpl, data, false)
			return
		}
		if len(body) > messageBodyMax {
			setToastLocked(store, p.ID, fmt.Sprintf("Message too long (max %d).", messageBodyMax))
			renderActionLikeResponse(w, tmpl, data, false)
			return
		}
		if sealed && p.Gold < sealedMessageCost {
			setToastLocked(store, p.ID, fmt.Sprintf("Need %dg to seal a missive.", sealedMessageCost))
			renderActionLikeResponse(w, tmpl, data, false)
			return
		}

		store.LastMessageAt[p.ID] = now
		if sealed {
			p.Gold -= sealedMessageCost
		}
		addDiplomacyMessageLocked(store, DiplomaticMessage{
			FromPlayerID: p.ID,
			FromName:     p.Name,
			ToPlayerID:   target.ID,
			ToName:       target.Name,
			Subject:      subject,
			Body:         body,
			At:           now,
			Sealed:       sealed,
		})
		setToastLocked(store, p.ID, fmt.Sprintf("Courier dispatched to %s.", target.Name))
		setToastLocked(store, target.ID, fmt.Sprintf("A courier arrives from %s.", p.Name))
		renderActionLikeResponse(w, tmpl, buildPageDataLocked(store, p.ID, true), false)
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
		csrfToken, err := issueAdminSessionCookies(w, r)
		if err != nil {
			http.Error(w, "failed to initialize admin session", http.StatusInternalServerError)
			return
		}

		store.mu.Lock()
		defer store.mu.Unlock()

		openContracts := 0
		acceptedContracts := 0
		failedContracts := 0
		for _, c := range store.Contracts {
			switch c.Status {
			case "Issued":
				openContracts++
			case "Accepted":
				acceptedContracts++
			case "Failed":
				failedContracts++
			}
		}
		players := make([]*Player, 0, len(store.Players))
		for _, p := range store.Players {
			players = append(players, p)
		}
		sort.Slice(players, func(i, j int) bool {
			if players[i].Heat != players[j].Heat {
				return players[i].Heat > players[j].Heat
			}
			return players[i].Name < players[j].Name
		})
		if len(players) > 8 {
			players = players[:8]
		}
		online := onlinePlayersLocked(store, time.Now().UTC())
		diag := buildAdminDiagnosticsLocked(store, time.Now().UTC())

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, "<!DOCTYPE html><html><head><meta charset=\"utf-8\"><title>Admin</title><style>body{font-family:ui-sans-serif,system-ui;background:#0b0f14;color:#e5ecf4;padding:24px;line-height:1.45}.muted{color:#9aa8bb}.row{display:flex;gap:12px;flex-wrap:wrap;margin:12px 0}.card{background:#121923;border:1px solid #2a3442;padding:12px;border-radius:8px;min-width:140px}h1,h2,h3{margin:0 0 8px}pre{background:#121923;border:1px solid #2a3442;padding:12px;border-radius:8px;overflow:auto}button{background:#1f6feb;color:#fff;border:0;padding:8px 12px;border-radius:6px;margin-right:8px;cursor:pointer}button.warn{background:#a11f32}input,select{background:#0e141d;color:#e5ecf4;border:1px solid #2a3442;border-radius:6px;padding:6px 8px;margin-right:8px}table{border-collapse:collapse;width:100%;max-width:720px;background:#121923;border:1px solid #2a3442;border-radius:8px;overflow:hidden}th,td{border-bottom:1px solid #2a3442;padding:8px;text-align:left}th{background:#0e141d}</style></head><body>")
		_, _ = fmt.Fprintf(w, "<h1>Admin</h1><p>Persistent realm state enabled via DB_DIALECT.</p>")
		_, _ = fmt.Fprintf(w, "<p><a href=\"/admin/state\" style=\"color:#9fc1ff\">Download state snapshot (JSON)</a></p>")
		_, _ = fmt.Fprintf(w, "<div class=\"row\"><div class=\"card\"><div class=\"muted\">Day</div><div>%d %s</div></div><div class=\"card\"><div class=\"muted\">Ticks</div><div>%d</div></div><div class=\"card\"><div class=\"muted\">Players</div><div>%d</div></div><div class=\"card\"><div class=\"muted\">Online</div><div>%d</div></div><div class=\"card\"><div class=\"muted\">Contracts</div><div>%d open / %d accepted</div></div><div class=\"card\"><div class=\"muted\">Failures</div><div>%d</div></div></div>",
			store.World.DayNumber, template.HTMLEscapeString(store.World.Subphase), store.TickCount, len(store.Players), len(online), openContracts, acceptedContracts, failedContracts)
		_, _ = fmt.Fprintf(w, "<h2>Controls</h2><form method=\"post\" action=\"/admin/tick\"><input type=\"hidden\" name=\"csrf_token\" value=\"%s\"><label for=\"tick_count\">Advance ticks:</label><input id=\"tick_count\" name=\"tick_count\" type=\"number\" min=\"1\" max=\"%d\" value=\"1\"><button type=\"submit\">Run</button></form>", template.HTMLEscapeString(csrfToken), adminMaxManualTicks)
		_, _ = fmt.Fprintf(w, "<form method=\"post\" action=\"/admin/reset\" style=\"margin-top:10px\"><input type=\"hidden\" name=\"csrf_token\" value=\"%s\"><label for=\"confirm_reset\">Type %q:</label><input id=\"confirm_reset\" name=\"confirm\" type=\"text\" autocomplete=\"off\"><button class=\"warn\" type=\"submit\">Reset World</button></form>", template.HTMLEscapeString(csrfToken), adminResetConfirmPhrase)
		_, _ = fmt.Fprintf(w, "<p class=\"muted\">Reset is destructive and clears players, contracts, intel, and history.</p>")
		_, _ = fmt.Fprintf(w, "<h2>Anomaly &amp; Balance Summary</h2><pre>Total players: %d (online %d, traveling %d)\nEconomy: gold=%d grain=%d avg_gold=%d avg_grain=%d\nStanding: avg_rep=%d avg_heat=%d hottest=%s (%d)\nContracts: issued=%d accepted=%d fulfilled=%d failed=%d overdue_active=%d anomalies=%d\nDebt pressure: overdue_loans=%d overdue_obligations=%d\nWorld pressure: %s\nAlerts (%d):\n",
			diag.TotalPlayers, diag.OnlinePlayers, diag.TravelingPlayers,
			diag.TotalGold, diag.TotalGrain, diag.AvgGoldPerPlayer, diag.AvgGrainPerPlayer,
			diag.AvgRep, diag.AvgHeat, template.HTMLEscapeString(diag.HottestPlayer), diag.HottestHeat,
			diag.ContractsIssued, diag.ContractsAccepted, diag.ContractsFulfilled, diag.ContractsFailed, diag.OverdueActiveContracts, diag.ContractStateAnomalies,
			diag.OverdueActiveLoans, diag.OverdueOpenObligations, template.HTMLEscapeString(diag.WorldPressureLevel), diag.AlertCount)
		for _, alert := range diag.Alerts {
			_, _ = fmt.Fprintf(w, " - %s\n", template.HTMLEscapeString(alert))
		}
		_, _ = fmt.Fprintf(w, "</pre>")
		_, _ = fmt.Fprintf(w, "<h2>Hot Players</h2><table><thead><tr><th>Name</th><th>Heat</th><th>Rep</th><th>Gold</th><th>Travel</th></tr></thead><tbody>")
		for _, p := range players {
			travel := "No"
			if p.TravelTicksLeft > 0 {
				travel = fmt.Sprintf("Yes (%d)", p.TravelTicksLeft)
			}
			_, _ = fmt.Fprintf(w, "<tr><td>%s</td><td>%d</td><td>%d</td><td>%d</td><td>%s</td></tr>",
				template.HTMLEscapeString(p.Name), p.Heat, p.Rep, p.Gold, template.HTMLEscapeString(travel))
		}
		_, _ = fmt.Fprintf(w, "</tbody></table>")

		_, _ = fmt.Fprintf(w, "<h2>World</h2><pre>%+v</pre>", store.World)
		_, _ = fmt.Fprintf(w, "<h2>Active Crisis</h2><pre>%+v</pre>", store.ActiveCrisis)
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

	mux.HandleFunc("/admin/state", func(w http.ResponseWriter, r *http.Request) {
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

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		payload := map[string]any{
			"generated_at":  time.Now().UTC().Format(time.RFC3339),
			"tick_count":    store.TickCount,
			"world":         store.World,
			"active_crisis": store.ActiveCrisis,
			"diagnostics":   buildAdminDiagnosticsLocked(store, time.Now().UTC()),
			"counts": map[string]int{
				"players":      len(store.Players),
				"contracts":    len(store.Contracts),
				"events":       len(store.Events),
				"messages":     len(store.Messages),
				"chat":         len(store.Chat),
				"projects":     len(store.Projects),
				"warrants":     len(store.Warrants),
				"permits":      len(store.Permits),
				"loans":        len(store.Loans),
				"obligations":  len(store.Obligations),
				"rumors":       len(store.Rumors),
				"evidence":     len(store.Evidence),
				"scry_reports": len(store.ScryReports),
				"intercepts":   len(store.Intercepts),
			},
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		if err := enc.Encode(payload); err != nil {
			http.Error(w, "failed to encode state", http.StatusInternalServerError)
			return
		}
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
		if !parsePostFormLimited(w, r, maxFormBodyBytes) {
			return
		}
		if !hasValidAdminHeaderToken(r) && !validateAdminCSRF(r) {
			http.Error(w, "invalid csrf token", http.StatusForbidden)
			return
		}
		store.mu.Lock()
		defer store.mu.Unlock()
		defer store.persistLocked()
		n := 1
		if raw := strings.TrimSpace(r.FormValue("tick_count")); raw != "" {
			if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
				n = parsed
			}
		}
		if n > adminMaxManualTicks {
			n = adminMaxManualTicks
		}
		now := time.Now().UTC()
		for i := 0; i < n; i++ {
			runWorldTickLocked(store, now)
		}
		store.LastTickAt = now
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
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
		if !parsePostFormLimited(w, r, maxFormBodyBytes) {
			return
		}
		if !hasValidAdminHeaderToken(r) && !validateAdminCSRF(r) {
			http.Error(w, "invalid csrf token", http.StatusForbidden)
			return
		}
		store.mu.Lock()
		defer store.mu.Unlock()
		defer store.persistLocked()
		if strings.TrimSpace(r.FormValue("confirm")) != adminResetConfirmPhrase {
			http.Error(w, "missing reset confirmation phrase", http.StatusBadRequest)
			return
		}
		resetStoreLocked(store)
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
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
			WardNetworkTicks:       0,
			Situation:              deriveSituation("Stable", "Calm"),
		},
		Players:           map[string]*Player{},
		Contracts:         map[string]*Contract{},
		Institutions:      map[string]*Institution{},
		Seats:             map[string]*Seat{},
		Policies:          PolicyState{TaxRatePct: 0},
		Rumors:            map[int64]*Rumor{},
		Evidence:          map[int64]*Evidence{},
		ScryReports:       map[int64]*ScryReport{},
		Intercepts:        map[int64]*InterceptedMessage{},
		Loans:             map[string]*Loan{},
		Obligations:       map[string]*Obligation{},
		Permits:           map[string]*Permit{},
		Warrants:          map[string]*Warrant{},
		Relics:            map[int64]*Relic{},
		Projects:          map[string]*Project{},
		ActiveCrisis:      nil,
		Events:            []Event{},
		Chat:              []ChatMessage{},
		Messages:          []DiplomaticMessage{},
		LastDailyTickDate: "",
		LastTickAt:        now,
		TickEvery:         60 * time.Second,
		LastChatAt:        map[string]time.Time{},
		LastMessageAt:     map[string]time.Time{},
		LastActionAt:      map[string]time.Time{},
		LastDeliverAt:     map[string]time.Time{},
		LastInvestigateAt: map[string]int64{},
		LastSeatActionAt:  map[string]int64{},
		LastIntelActionAt: map[string]int64{},
		LastFieldworkAt:   map[string]int64{},
		DailyActionDate:   map[string]string{},
		DailyHighImpactN:  map[string]int{},
		ToastByPlayer:     map[string]string{},
		LastCleanupDate:   "",
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
		WardNetworkTicks:       0,
		Situation:              deriveSituation("Stable", "Calm"),
	}
	s.Players = map[string]*Player{}
	s.Contracts = map[string]*Contract{}
	s.Institutions = map[string]*Institution{}
	s.Seats = map[string]*Seat{}
	s.Policies = PolicyState{TaxRatePct: 0}
	s.Rumors = map[int64]*Rumor{}
	s.Evidence = map[int64]*Evidence{}
	s.ScryReports = map[int64]*ScryReport{}
	s.Intercepts = map[int64]*InterceptedMessage{}
	s.Loans = map[string]*Loan{}
	s.Obligations = map[string]*Obligation{}
	s.Permits = map[string]*Permit{}
	s.Warrants = map[string]*Warrant{}
	s.Relics = map[int64]*Relic{}
	s.Projects = map[string]*Project{}
	s.ActiveCrisis = nil
	s.Events = []Event{}
	s.Chat = []ChatMessage{}
	s.Messages = []DiplomaticMessage{}
	s.NextEventID = 0
	s.NextContractID = 0
	s.NextChatID = 0
	s.NextMessageID = 0
	s.NextProjectID = 0
	s.NextRelicID = 0
	s.NextScryID = 0
	s.NextInterceptID = 0
	s.LastDailyTickDate = ""
	s.LastTickAt = now
	s.TickCount = 0
	s.LastChatAt = map[string]time.Time{}
	s.LastMessageAt = map[string]time.Time{}
	s.LastActionAt = map[string]time.Time{}
	s.LastDeliverAt = map[string]time.Time{}
	s.LastInvestigateAt = map[string]int64{}
	s.LastSeatActionAt = map[string]int64{}
	s.LastIntelActionAt = map[string]int64{}
	s.LastFieldworkAt = map[string]int64{}
	s.DailyActionDate = map[string]string{}
	s.DailyHighImpactN = map[string]int{}
	s.ToastByPlayer = map[string]string{}
	s.LastCleanupDate = ""
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
				store.persistLocked()
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
	processProjectTickLocked(store, now)
	processPlayerTickLocked(store, now)
	processTravelTickLocked(store, now)
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

		if c.Type == "Bounty" {
			c.DeadlineTicks--
			if c.DeadlineTicks <= 0 {
				c.Status = "Failed"
				if c.OwnerPlayerID != "" {
					if owner := store.Players[c.OwnerPlayerID]; owner != nil {
						owner.Rep = clampInt(owner.Rep-3, -100, 100)
					}
				}
				addEventLocked(store, Event{Type: "Law", Severity: 2, Text: fmt.Sprintf("A bounty on [%s] lapses without arrests.", c.TargetName), At: now})
			}
			continue
		}
		if c.Type == "Supply" {
			c.DeadlineTicks--
			if c.DeadlineTicks <= 0 {
				c.Status = "Failed"
				if issuer := store.Players[c.IssuerPlayerID]; issuer != nil {
					refund := c.RewardGold / 2
					if refund > 0 {
						issuer.Gold += refund
					}
				}
				addEventLocked(store, Event{Type: "Contract", Severity: 2, Text: "A supply contract expires; only half the escrow is recovered.", At: now})
			}
			continue
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

	processCrisisTickLocked(store, now)

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
	for _, target := range store.Players {
		if target == nil {
			continue
		}
		if !isWantedHeat(target.Heat) {
			continue
		}
		if now.Sub(target.LastSeen) > inactiveWindow {
			continue
		}
		if hasActiveBountyForTargetLocked(store, target.ID) {
			continue
		}
		issueBountyContractLocked(store, target, bountyDeadlineTicks)
		addEventLocked(store, Event{Type: "Law", Severity: 3, Text: fmt.Sprintf("[City Watch] posts a bounty on [%s].", target.Name), At: now})
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
	store.Seats["watch_commander"] = &Seat{
		ID:              "watch_commander",
		Name:            "Commander of the Watch",
		InstitutionID:   "city_authority",
		HolderName:      "Marshal Dain (NPC)",
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
	if store.World.WardNetworkTicks > 0 {
		store.World.WardNetworkTicks--
		if store.World.WardNetworkTicks == 0 {
			addEventLocked(store, Event{Type: "Doctrine", Severity: 1, Text: "Ward lanterns gutter; the veil thins.", At: now})
		}
	}
	for playerID, permit := range store.Permits {
		if permit == nil {
			delete(store.Permits, playerID)
			continue
		}
		permit.TicksLeft--
		if permit.TicksLeft <= 0 {
			delete(store.Permits, playerID)
			addEventLocked(store, Event{Type: "Policy", Severity: 1, Text: fmt.Sprintf("Permit for [%s] expires.", permit.PlayerName), At: now})
		}
	}
	for playerID, warrant := range store.Warrants {
		if warrant == nil {
			delete(store.Warrants, playerID)
			continue
		}
		warrant.TicksLeft--
		if warrant.TicksLeft <= 0 {
			delete(store.Warrants, playerID)
			addEventLocked(store, Event{Type: "Law", Severity: 1, Text: fmt.Sprintf("Warrant on [%s] expires.", warrant.PlayerName), At: now})
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
	case "watch_commander":
		return "Marshal Dain (NPC)"
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

func permitForPlayerLocked(store *Store, playerID string) *Permit {
	permit := store.Permits[playerID]
	if permit == nil || permit.TicksLeft <= 0 {
		return nil
	}
	return permit
}

func hasActivePermitLocked(store *Store, playerID string) bool {
	return permitForPlayerLocked(store, playerID) != nil
}

func warrantForPlayerLocked(store *Store, playerID string) *Warrant {
	warrant := store.Warrants[playerID]
	if warrant == nil || warrant.TicksLeft <= 0 {
		return nil
	}
	return warrant
}

func hasActiveWarrantLocked(store *Store, playerID string) bool {
	return warrantForPlayerLocked(store, playerID) != nil
}

func processIntelTickLocked(store *Store, now time.Time) {
	warded := store.World.WardNetworkTicks > 0
	for id, r := range store.Rumors {
		spreadGain := maxInt(1, r.Credibility/3)
		if warded {
			spreadGain = maxInt(0, spreadGain-1)
			r.Decay--
		}
		r.Spread += spreadGain
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

	for id, report := range store.ScryReports {
		if report.ExpiryTick <= store.TickCount {
			delete(store.ScryReports, id)
		}
	}

	for id, intercept := range store.Intercepts {
		if intercept.ExpiryTick <= store.TickCount {
			delete(store.Intercepts, id)
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

	overdueThisTick := 0
	for _, ob := range store.Obligations {
		if ob.Status != "Open" || ob.DueTick > store.TickCount {
			continue
		}
		ob.Status = "Overdue"
		ob.TerminalAt = now
		overdueThisTick++
		debtor := store.Players[ob.DebtorPlayerID]
		creditor := store.Players[ob.CreditorPlayerID]
		if debtor != nil {
			debtor.Rep = clampInt(debtor.Rep-(1+ob.Severity), -100, 100)
			debtor.Heat = clampInt(debtor.Heat+maxInt(1, ob.Severity/2), 0, 20)
		}
		if creditor != nil {
			creditor.Rep = clampInt(creditor.Rep+1, -100, 100)
		}
		addEventLocked(store, Event{
			Type:     "Finance",
			Severity: 2,
			Text:     fmt.Sprintf("[%s] falls behind on a favor owed to [%s].", ob.DebtorName, ob.CreditorName),
			At:       now,
		})
	}
	if overdueThisTick > 0 {
		store.World.UnrestValue = clampInt(store.World.UnrestValue+overdueThisTick, 0, 100)
		store.World.UnrestTier = unrestTierFromValue(store.World.UnrestValue)
	}
}

func processProjectTickLocked(store *Store, now time.Time) {
	if len(store.Projects) == 0 {
		return
	}
	for id, proj := range store.Projects {
		proj.TicksLeft--
		if proj.TicksLeft > 0 {
			continue
		}
		def, ok := projectDefinitionByType(proj.Type)
		if ok {
			if def.GrainDelta != 0 {
				applyGrainSupplyDeltaLocked(store, now, def.GrainDelta)
			}
			if def.UnrestDelta != 0 {
				store.World.UnrestValue = clampInt(store.World.UnrestValue+def.UnrestDelta, 0, 100)
				store.World.UnrestTier = unrestTierFromValue(store.World.UnrestValue)
			}
			if owner := store.Players[proj.OwnerPlayerID]; owner != nil {
				if def.RepDelta != 0 {
					owner.Rep = clampInt(owner.Rep+def.RepDelta, -100, 100)
				}
				if def.HeatDelta != 0 {
					owner.Heat = clampInt(owner.Heat+def.HeatDelta, 0, 20)
				}
			}
			if def.WardNetworkTicks > 0 {
				if store.World.WardNetworkTicks < def.WardNetworkTicks {
					store.World.WardNetworkTicks = def.WardNetworkTicks
				}
			}
			addEventLocked(store, Event{
				Type:     "Civic",
				Severity: 2,
				Text:     fmt.Sprintf("%s completes; %s", proj.Name, projectEffectNote(def)),
				At:       now,
			})
		} else {
			addEventLocked(store, Event{
				Type:     "Civic",
				Severity: 1,
				Text:     fmt.Sprintf("%s completes and ripples through the city.", proj.Name),
				At:       now,
			})
		}
		delete(store.Projects, id)
	}
}

func startCrisisLocked(store *Store, def CrisisDefinition, now time.Time) {
	store.ActiveCrisis = &Crisis{
		Type:        def.Type,
		Name:        def.Name,
		Description: def.Description,
		Severity:    def.BaseSeverity,
		TicksLeft:   def.DurationTicks,
		TotalTicks:  def.DurationTicks,
	}
	addEventLocked(store, Event{
		Type:     "Crisis",
		Severity: 3,
		Text:     fmt.Sprintf("%s erupts. %s", def.Name, def.Description),
		At:       now,
	})
}

func maybeStartCrisisLocked(store *Store, now time.Time) {
	if store.ActiveCrisis != nil {
		return
	}
	chance := 4
	if store.World.UnrestTier == "Rioting" || store.World.UnrestTier == "Unstable" {
		chance += 8
	}
	if store.World.GrainTier == "Critical" || store.World.GrainTier == "Scarce" {
		chance += 6
	}
	if store.rng.Intn(100) >= chance {
		return
	}
	defs := crisisDefinitions()
	if len(defs) == 0 {
		return
	}
	def := defs[store.rng.Intn(len(defs))]
	startCrisisLocked(store, def, now)
}

func resolveCrisisLocked(store *Store, def CrisisDefinition, now time.Time, mitigated bool, resolver *Player) {
	if mitigated {
		if resolver != nil {
			addEventLocked(store, Event{
				Type:     "Crisis",
				Severity: 2,
				Text:     fmt.Sprintf("[%s] contains %s.", resolver.Name, def.Name),
				At:       now,
			})
		} else {
			addEventLocked(store, Event{
				Type:     "Crisis",
				Severity: 2,
				Text:     fmt.Sprintf("%s abates after emergency measures.", def.Name),
				At:       now,
			})
		}
		store.ActiveCrisis = nil
		return
	}
	if def.FailureUnrestDelta != 0 {
		store.World.UnrestValue = clampInt(store.World.UnrestValue+def.FailureUnrestDelta, 0, 100)
		store.World.UnrestTier = unrestTierFromValue(store.World.UnrestValue)
	}
	if def.FailureGrainDelta != 0 {
		applyGrainSupplyDeltaLocked(store, now, def.FailureGrainDelta)
	}
	addEventLocked(store, Event{
		Type:     "Crisis",
		Severity: 4,
		Text:     fmt.Sprintf("%s burns out of control; the city reels.", def.Name),
		At:       now,
	})
	store.ActiveCrisis = nil
}

func processCrisisTickLocked(store *Store, now time.Time) {
	if store.ActiveCrisis == nil {
		maybeStartCrisisLocked(store, now)
		return
	}
	crisis := store.ActiveCrisis
	def, ok := crisisDefinitionByType(crisis.Type)
	if !ok {
		store.ActiveCrisis = nil
		return
	}
	if crisis.TicksLeft <= 0 {
		resolveCrisisLocked(store, def, now, crisis.Mitigated, nil)
		return
	}
	if def.TickUnrestDelta != 0 {
		store.World.UnrestValue += def.TickUnrestDelta * maxInt(1, crisis.Severity)
	}
	if def.TickGrainDelta != 0 {
		applyGrainSupplyDeltaLocked(store, now, def.TickGrainDelta*maxInt(1, crisis.Severity))
	}
	crisis.TicksLeft--
	if crisis.TicksLeft <= 0 {
		resolveCrisisLocked(store, def, now, crisis.Mitigated, nil)
	}
}

func processPlayerTickLocked(store *Store, now time.Time) {
	for _, p := range store.Players {
		if p.RiteImmunityTicks > 0 {
			p.RiteImmunityTicks--
		}
		if p.BribeAccessTicks > 0 {
			p.BribeAccessTicks--
			if p.BribeAccessTicks == 0 {
				addEventLocked(store, Event{Type: "Institution", Severity: 1, Text: fmt.Sprintf("Officials stop extending favors to [%s].", p.Name), At: now})
			}
		}
	}
}

func processTravelTickLocked(store *Store, now time.Time) {
	for _, p := range store.Players {
		if p.TravelTicksLeft <= 0 {
			continue
		}
		p.TravelTicksLeft--
		if p.TravelTicksLeft > 0 {
			continue
		}
		destID := p.TravelToID
		p.LocationID = destID
		p.TravelToID = ""
		p.TravelTotalTicks = 0
		destName := locationName(destID)
		addEventLocked(store, Event{
			Type:     "Travel",
			Severity: 1,
			Text:     fmt.Sprintf("[%s] arrives at %s.", p.Name, destName),
			At:       now,
		})
		setToastLocked(store, p.ID, fmt.Sprintf("Arrived at %s.", destName))
	}
}

func fieldworkCooldownRemaining(store *Store, playerID string) int {
	lastTick, ok := store.LastFieldworkAt[playerID]
	if !ok {
		return 0
	}
	diff := int(store.TickCount - lastTick)
	if diff < fieldworkCooldownTicks {
		return fieldworkCooldownTicks - diff
	}
	return 0
}

func locationDefinitions() []LocationDef {
	return []LocationDef{
		{ID: locationCapital, Name: "Black Granary (Capital)", Description: "The granary citadel and its surrounding markets."},
		{ID: locationHarbor, Name: "Harbor Ward", Description: "Salt air, cargo manifests, and merchant seals."},
		{ID: locationFrontier, Name: "Frontier Village", Description: "Wind-scoured outpost clinging to the trade road."},
		{ID: locationRuins, Name: "Haunted Ruins", Description: "A broken keep where relics and rumors linger."},
	}
}

func locationByID(id string) (LocationDef, bool) {
	for _, def := range locationDefinitions() {
		if def.ID == id {
			return def, true
		}
	}
	return LocationDef{}, false
}

func locationName(id string) string {
	if def, ok := locationByID(id); ok {
		return def.Name
	}
	return "Unknown"
}

func iconAsset(artist, name string) string {
	return fmt.Sprintf("/assets/icons/ffffff/transparent/1x1/%s/%s.png", artist, name)
}

func locationIcon(id string) (string, string) {
	switch id {
	case locationCapital:
		return iconAsset("delapouite", "castle"), "gold"
	case locationHarbor:
		return iconAsset("delapouite", "lighthouse"), "teal"
	case locationFrontier:
		return iconAsset("delapouite", "village"), "amber"
	case locationRuins:
		return iconAsset("delapouite", "crystal-shrine"), "violet"
	default:
		return iconAsset("delapouite", "earth-america"), "blue"
	}
}

func contractTypeIcon(contractType string) (string, string) {
	switch contractType {
	case "Emergency":
		return iconAsset("delapouite", "factory-arm"), "amber"
	case "Smuggling":
		return iconAsset("delapouite", "lighthouse"), "teal"
	case "Bounty":
		return iconAsset("delapouite", "sword-altar"), "red"
	case "Supply":
		return iconAsset("delapouite", "warehouse"), "lime"
	default:
		return iconAsset("delapouite", "perspective-dice-six"), "blue"
	}
}

func travelTicksBetween(from, to string) int {
	if from == "" || to == "" || from == to {
		return 0
	}
	travel := map[string]int{
		locationCapital + ":" + locationHarbor:   1,
		locationHarbor + ":" + locationCapital:   1,
		locationCapital + ":" + locationFrontier: 2,
		locationFrontier + ":" + locationCapital: 2,
		locationHarbor + ":" + locationFrontier:  2,
		locationFrontier + ":" + locationHarbor:  2,
		locationFrontier + ":" + locationRuins:   2,
		locationRuins + ":" + locationFrontier:   2,
		locationCapital + ":" + locationRuins:    3,
		locationRuins + ":" + locationCapital:    3,
		locationHarbor + ":" + locationRuins:     3,
		locationRuins + ":" + locationHarbor:     3,
	}
	if ticks, ok := travel[from+":"+to]; ok {
		return ticks
	}
	return 2
}

func relicDefinitions() []RelicDefinition {
	return []RelicDefinition{
		{Name: "Warding Charm", Effect: "heat", Power: 2},
		{Name: "Oathstone Shard", Effect: "rep", Power: 2},
		{Name: "Cinder Coin", Effect: "gold", Power: 6},
		{Name: "Whispered Sigil", Effect: "rumor", Power: 2},
		{Name: "Harvest Token", Effect: "grain", Power: 2},
	}
}

func randomRelicDefinition(rng *mathrand.Rand) RelicDefinition {
	defs := relicDefinitions()
	if len(defs) == 0 {
		return RelicDefinition{Name: "Unnamed Relic", Effect: "gold", Power: 1}
	}
	return defs[rng.Intn(len(defs))]
}

func relicEffectLabel(relic *Relic) string {
	if relic == nil {
		return ""
	}
	switch relic.Effect {
	case "heat":
		return fmt.Sprintf("Effect: -%d heat", relic.Power)
	case "rep":
		return fmt.Sprintf("Effect: +%d reputation", relic.Power)
	case "gold":
		return fmt.Sprintf("Effect: +%dg", relic.Power)
	case "rumor":
		return fmt.Sprintf("Effect: +%d rumors", relic.Power)
	case "grain":
		return fmt.Sprintf("Effect: +%d sacks", relic.Power)
	default:
		return "Effect: unknown"
	}
}

func addRelicLocked(store *Store, owner *Player, def RelicDefinition, now time.Time) *Relic {
	if owner == nil {
		return nil
	}
	store.NextRelicID++
	id := store.NextRelicID
	relic := &Relic{
		ID:              id,
		Name:            def.Name,
		Effect:          def.Effect,
		Power:           def.Power,
		OwnerPlayerID:   owner.ID,
		OwnerName:       owner.Name,
		Status:          relicStatusUnappraised,
		FoundAtTick:     store.TickCount,
		AppraisedAtTick: 0,
	}
	store.Relics[id] = relic
	addEventLocked(store, Event{
		Type:     "Fieldwork",
		Severity: 2,
		Text:     fmt.Sprintf("[%s] recovers a sealed relic from the ruins.", owner.Name),
		At:       now,
	})
	return relic
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

func addEvidenceLocked(store *Store, source *Player, target *Player, topic string, strength int, ttlTicks int64, forged bool) {
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
		Forged:         forged,
	}
}

func addScryReportLocked(store *Store, owner *Player, target *Player) {
	if owner == nil || target == nil {
		return
	}
	store.NextScryID++
	id := store.NextScryID
	store.ScryReports[id] = &ScryReport{
		ID:              id,
		OwnerPlayerID:   owner.ID,
		TargetPlayerID:  target.ID,
		TargetName:      target.Name,
		LocationID:      target.LocationID,
		TravelToID:      target.TravelToID,
		TravelTicksLeft: target.TravelTicksLeft,
		Rep:             target.Rep,
		Heat:            target.Heat,
		Gold:            target.Gold,
		Grain:           target.Grain,
		ExpiryTick:      store.TickCount + scryReportDurationTicks,
	}
}

func addInterceptLocked(store *Store, owner *Player, msg *DiplomaticMessage) {
	if owner == nil || msg == nil {
		return
	}
	store.NextInterceptID++
	id := store.NextInterceptID
	store.Intercepts[id] = &InterceptedMessage{
		ID:            id,
		OwnerPlayerID: owner.ID,
		OwnerName:     owner.Name,
		FromName:      msg.FromName,
		ToName:        msg.ToName,
		Subject:       msg.Subject,
		Body:          msg.Body,
		At:            msg.At,
		ExpiryTick:    store.TickCount + interceptDurationTicks,
		Sealed:        msg.Sealed,
	}
}

func mostRecentDiplomaticMessageForTarget(store *Store, targetID string) *DiplomaticMessage {
	if store == nil || targetID == "" {
		return nil
	}
	for i := len(store.Messages) - 1; i >= 0; i-- {
		msg := store.Messages[i]
		if msg.FromPlayerID == targetID || msg.ToPlayerID == targetID {
			return &store.Messages[i]
		}
	}
	return nil
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
		DueTick:          store.TickCount + obligationDueTicks,
		Status:           "Open",
		TerminalAt:       time.Time{},
	}
}

func obligationCost(severity int) int {
	return clampInt(severity*3, 3, 20)
}

func applyBountyResolutionLocked(store *Store, hunter, target *Player, ev *Evidence, now time.Time) {
	if store == nil || hunter == nil || target == nil {
		return
	}
	strength := 0
	if ev != nil {
		strength = ev.Strength
	}
	heatDrop := clampInt(2+strength/2, 2, 6)
	repDrop := clampInt(2+strength/3, 2, 6)
	target.Heat = maxInt(0, target.Heat-heatDrop)
	target.Rep = clampInt(target.Rep-repDrop, -100, 100)
	addEventLocked(store, Event{
		Type:     "Law",
		Severity: 3,
		Text:     fmt.Sprintf("[%s] delivers evidence; the Watch moves on [%s].", hunter.Name, target.Name),
		At:       now,
	})
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
	loan.TerminalAt = now
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
	Action       string
	ContractID   string
	Stance       string
	TargetID     string
	RelicID      string
	Claim        string
	Topic        string
	LoanID       string
	ObligationID string
	ProjectType  string
	LocationID   string
	Amount       int
	Sacks        int
	Reward       int
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
	if p.TravelTicksLeft > 0 && action != "travel" {
		setToastLocked(store, p.ID, fmt.Sprintf("You are en route to %s.", locationName(p.TravelToID)))
		return
	}
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
		if c.IssuerPlayerID != "" && c.IssuerPlayerID == p.ID {
			setToastLocked(store, p.ID, "You cannot accept your own contract.")
			return
		}
		hasBribedAccess := p.BribeAccessTicks > 0
		if c.Type == "Smuggling" && p.Rep < -50 {
			setToastLocked(store, p.ID, "Your reputation blocks smuggling contracts.")
			return
		}
		if c.Type == "Smuggling" && store.Policies.SmugglingEmbargoTicks > 0 && !playerHoldsSeatLocked(store, p.ID, "harbor_master") && !hasBribedAccess {
			setToastLocked(store, p.ID, "Smuggling is under embargo.")
			return
		}
		if c.Type == "Emergency" && store.Policies.PermitRequiredHighRisk && p.Rep < 20 && !playerHoldsSeatLocked(store, p.ID, "harbor_master") && !hasActivePermitLocked(store, p.ID) && !hasBribedAccess {
			setToastLocked(store, p.ID, "Permit required for emergency contracts.")
			return
		}
		if c.Type == "Bounty" && c.TargetPlayerID == p.ID {
			setToastLocked(store, p.ID, "You are the target of that bounty.")
			return
		}
		if playerAcceptedCountLocked(store, p.ID) >= 1 {
			setToastLocked(store, p.ID, "You can hold only one active contract.")
			return
		}
		c.Status = "Accepted"
		c.OwnerPlayerID = p.ID
		c.OwnerName = p.Name
		if c.Type != "Bounty" && c.Type != "Supply" {
			c.Stance = normalizeContractStance(stance)
		} else {
			c.Stance = ""
		}
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
		if c.Type == "Supply" {
			if c.SupplySacks <= 0 {
				setToastLocked(store, p.ID, "Supply contract has no defined quantity.")
				return
			}
			if p.Grain < c.SupplySacks {
				setToastLocked(store, p.ID, fmt.Sprintf("Need %d sacks to fulfill this contract.", c.SupplySacks))
				return
			}
			p.Grain -= c.SupplySacks
			applyGrainSupplyDeltaLocked(store, now, c.SupplySacks*grainUnitPerSack)
			finalizeDeliveredContractLocked(store, p, c, now)
			if issuer := store.Players[c.IssuerPlayerID]; issuer != nil && issuer.ID != p.ID {
				issuer.Rep = clampInt(issuer.Rep+1, -100, 100)
			}
			store.World.UnrestValue = clampInt(store.World.UnrestValue-4, 0, 100)
			store.World.UnrestTier = unrestTierFromValue(store.World.UnrestValue)
			addEventLocked(store, Event{Type: "Contract", Severity: 2, Text: fmt.Sprintf("[%s] delivers supplies on a patron's contract.", p.Name), At: now})
			setToastLocked(store, p.ID, "Supplies delivered.")
			return
		}
		if c.Type == "Bounty" {
			target := store.Players[c.TargetPlayerID]
			if target == nil {
				setToastLocked(store, p.ID, "Target no longer available.")
				return
			}
			ev := strongestEvidenceForLocked(store, p.ID, target.ID)
			required := c.BountyEvidence
			if required <= 0 {
				required = bountyEvidenceMin
			}
			if ev == nil || ev.Strength < required {
				setToastLocked(store, p.ID, fmt.Sprintf("Need evidence strength %d+ on target.", required))
				return
			}
			delete(store.Evidence, ev.ID)
			applyBountyResolutionLocked(store, p, target, ev, now)
			finalizeDeliveredContractLocked(store, p, c, now)
			setToastLocked(store, p.ID, "Bounty delivered.")
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
	case "post_supply":
		if hasActiveSupplyFromIssuerLocked(store, p.ID) {
			setToastLocked(store, p.ID, "You already have a supply contract active.")
			return
		}
		if in.Sacks <= 0 {
			setToastLocked(store, p.ID, "Choose a valid sack count.")
			return
		}
		if in.Reward <= 0 {
			setToastLocked(store, p.ID, "Choose a valid reward.")
			return
		}
		sacks := clampInt(in.Sacks, supplyContractMinSacks, supplyContractMaxSacks)
		reward := clampInt(in.Reward, supplyContractMinReward, supplyContractMaxReward)
		if p.Gold < reward {
			setToastLocked(store, p.ID, "Insufficient gold to escrow that reward.")
			return
		}
		p.Gold -= reward
		issueSupplyContractLocked(store, p, sacks, reward, supplyContractDeadlineTicks)
		addEventLocked(store, Event{Type: "Contract", Severity: 2, Text: fmt.Sprintf("[%s] posts a supply contract for %d sacks.", p.Name, sacks), At: now})
		setToastLocked(store, p.ID, "Supply contract posted.")
	case "cancel_contract":
		if c == nil || c.Status != "Issued" || c.IssuerPlayerID != p.ID {
			setToastLocked(store, p.ID, "Only the issuer can cancel an open contract.")
			return
		}
		refund := c.RewardGold
		if refund > 0 {
			p.Gold += refund
		}
		c.Status = "Cancelled"
		addEventLocked(store, Event{Type: "Contract", Severity: 1, Text: fmt.Sprintf("[%s] withdraws a supply contract.", p.Name), At: now})
		setToastLocked(store, p.ID, "Contract withdrawn.")
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
					addEvidenceLocked(store, p, target, chooseTopic(in.Topic, "corruption"), 5+maxInt(0, p.Rep/25), 5, false)
					setToastLocked(store, p.ID, "Your investigation found evidence.")
					break
				}
			}
			setToastLocked(store, p.ID, "Your investigation calmed the streets.")
		} else {
			addEventLocked(store, Event{Type: "Player", Severity: 1, Text: fmt.Sprintf("[%s] investigates rumors along the supply routes.", p.Name), At: now})
			setToastLocked(store, p.ID, "You find only fragments and gossip.")
		}
	case "forge_evidence":
		target := store.Players[in.TargetID]
		if target == nil || target.ID == p.ID {
			setToastLocked(store, p.ID, "Choose a valid target to forge evidence.")
			return
		}
		if tooSoonTick(store.LastIntelActionAt[p.ID], store.TickCount, 1) {
			setToastLocked(store, p.ID, "Forgery cooldown active.")
			return
		}
		if !consumeHighImpactBudgetLocked(store, p.ID, now) {
			setToastLocked(store, p.ID, "Daily cap reached for high-impact actions.")
			return
		}
		if p.Gold < forgeEvidenceCost {
			setToastLocked(store, p.ID, fmt.Sprintf("Need %dg to forge a dossier.", forgeEvidenceCost))
			return
		}
		store.LastIntelActionAt[p.ID] = store.TickCount
		p.Gold -= forgeEvidenceCost
		successChance := 55 + maxInt(0, p.Rep)/3
		if rollPercent(store.rng, minInt(successChance, 90)) {
			strength := clampInt(2+maxInt(0, p.Rep)/35, 2, 5)
			addEvidenceLocked(store, p, target, chooseTopic(in.Topic, "fraud"), strength, forgeEvidenceDurationTicks, true)
			addEventLocked(store, Event{
				Type:     "Intel",
				Severity: 2,
				Text:     fmt.Sprintf("[%s] circulates a forged dossier on [%s].", p.Name, target.Name),
				At:       now,
			})
			setToastLocked(store, p.ID, "Forgery completed. Evidence added to your dossier.")
		} else {
			p.Rep = clampInt(p.Rep-3, -100, 100)
			p.Heat = clampInt(p.Heat+2, 0, 20)
			addEventLocked(store, Event{
				Type:     "Intel",
				Severity: 2,
				Text:     fmt.Sprintf("[%s] is caught manufacturing evidence.", p.Name),
				At:       now,
			})
			setToastLocked(store, p.ID, "Forgery exposed; your reputation suffers.")
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
		ev := strongestEvidenceForLocked(store, p.ID, target.ID)
		if ev == nil {
			setToastLocked(store, p.ID, "You lack evidence on that target.")
			return
		}
		if !consumeHighImpactBudgetLocked(store, p.ID, now) {
			setToastLocked(store, p.ID, "Daily cap reached for high-impact actions.")
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
	case "scry_target":
		target := store.Players[in.TargetID]
		if target == nil || target.ID == p.ID {
			setToastLocked(store, p.ID, "Choose a valid target to scry.")
			return
		}
		if tooSoonTick(store.LastIntelActionAt[p.ID], store.TickCount, 1) {
			setToastLocked(store, p.ID, "Scrying cooldown active.")
			return
		}
		if !consumeHighImpactBudgetLocked(store, p.ID, now) {
			setToastLocked(store, p.ID, "Daily cap reached for high-impact actions.")
			return
		}
		store.LastIntelActionAt[p.ID] = store.TickCount
		if target.RiteImmunityTicks > 0 {
			p.Rep = clampInt(p.Rep-1, -100, 100)
			setToastLocked(store, p.ID, "Ritual wards deflect your scrying.")
			setToastLocked(store, target.ID, "Your wards shimmer; someone sought you through the veil.")
			return
		}
		successChance := 45 + maxInt(0, p.Rep)/4
		if store.World.WardNetworkTicks > 0 {
			successChance = maxInt(10, successChance-12)
		}
		if rollPercent(store.rng, minInt(successChance, 85)) {
			addScryReportLocked(store, p, target)
			addEventLocked(store, Event{
				Type:     "Intel",
				Severity: 2,
				Text:     fmt.Sprintf("[%s] completes a scrying report on [%s].", p.Name, target.Name),
				At:       now,
			})
			setToastLocked(store, p.ID, "Scrying report added to your dossier.")
		} else {
			p.Rep = clampInt(p.Rep-2, -100, 100)
			p.Heat = clampInt(p.Heat+1, 0, 20)
			setToastLocked(store, p.ID, "The scrying ritual falters and leaves traces.")
			setToastLocked(store, target.ID, "A scrying attempt brushes past your wards.")
		}
	case "intercept_courier":
		target := store.Players[in.TargetID]
		if target == nil || target.ID == p.ID {
			setToastLocked(store, p.ID, "Choose a valid courier target.")
			return
		}
		msg := mostRecentDiplomaticMessageForTarget(store, target.ID)
		if msg == nil {
			setToastLocked(store, p.ID, "No courier traffic to intercept.")
			return
		}
		if tooSoonTick(store.LastIntelActionAt[p.ID], store.TickCount, 1) {
			setToastLocked(store, p.ID, "Intercept cooldown active.")
			return
		}
		if !consumeHighImpactBudgetLocked(store, p.ID, now) {
			setToastLocked(store, p.ID, "Daily cap reached for high-impact actions.")
			return
		}
		store.LastIntelActionAt[p.ID] = store.TickCount
		successChance := 45 + maxInt(0, p.Rep)/3 + target.Heat/2 - maxInt(0, target.Rep)/6
		if store.World.WardNetworkTicks > 0 {
			successChance = maxInt(10, successChance-10)
		}
		if msg.Sealed {
			successChance = maxInt(10, successChance-sealedInterceptPenalty)
		}
		if rollPercent(store.rng, minInt(successChance, 85)) {
			addInterceptLocked(store, p, msg)
			addEventLocked(store, Event{
				Type:     "Intel",
				Severity: 2,
				Text:     fmt.Sprintf("[%s] intercepts a courier bound for [%s].", p.Name, target.Name),
				At:       now,
			})
			if msg.Sealed {
				setToastLocked(store, p.ID, "Courier intercepted; the seal holds.")
			} else {
				setToastLocked(store, p.ID, "Courier intercepted; missive logged.")
			}
		} else {
			p.Rep = clampInt(p.Rep-1, -100, 100)
			p.Heat = clampInt(p.Heat+1, 0, 20)
			setToastLocked(store, p.ID, "The courier slips past your agents.")
			setToastLocked(store, target.ID, "Your couriers report a near miss.")
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
			TerminalAt:       time.Time{},
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
			loan.TerminalAt = now
			return
		}
		lender.Gold -= loan.Principal
		p.Gold += loan.Principal
		loan.Status = "Active"
		loan.TerminalAt = time.Time{}
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
			loan.TerminalAt = now
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
	case "settle_obligation":
		ob := store.Obligations[in.ObligationID]
		if ob == nil || (ob.Status != "Open" && ob.Status != "Overdue") {
			setToastLocked(store, p.ID, "No outstanding obligation found.")
			return
		}
		if ob.DebtorPlayerID != p.ID {
			setToastLocked(store, p.ID, "Only the debtor can settle this.")
			return
		}
		cost := obligationCost(ob.Severity)
		if p.Gold < cost {
			setToastLocked(store, p.ID, fmt.Sprintf("Need %dg to settle.", cost))
			return
		}
		p.Gold -= cost
		if creditor := store.Players[ob.CreditorPlayerID]; creditor != nil {
			creditor.Gold += cost
			creditor.Rep = clampInt(creditor.Rep+1, -100, 100)
		}
		p.Rep = clampInt(p.Rep+2, -100, 100)
		p.Heat = maxInt(0, p.Heat-1)
		ob.Status = "Settled"
		ob.TerminalAt = now
		addEventLocked(store, Event{
			Type:     "Finance",
			Severity: 2,
			Text:     fmt.Sprintf("[%s] settles a favor owed to [%s].", ob.DebtorName, ob.CreditorName),
			At:       now,
		})
		setToastLocked(store, p.ID, "Obligation settled.")
	case "forgive_obligation":
		ob := store.Obligations[in.ObligationID]
		if ob == nil || (ob.Status != "Open" && ob.Status != "Overdue") {
			setToastLocked(store, p.ID, "No outstanding obligation found.")
			return
		}
		if ob.CreditorPlayerID != p.ID {
			setToastLocked(store, p.ID, "Only the creditor can forgive this.")
			return
		}
		ob.Status = "Forgiven"
		ob.TerminalAt = now
		p.Rep = clampInt(p.Rep+2, -100, 100)
		if debtor := store.Players[ob.DebtorPlayerID]; debtor != nil {
			debtor.Rep = clampInt(debtor.Rep+1, -100, 100)
		}
		addEventLocked(store, Event{
			Type:     "Finance",
			Severity: 1,
			Text:     fmt.Sprintf("[%s] forgives a favor owed by [%s].", ob.CreditorName, ob.DebtorName),
			At:       now,
		})
		setToastLocked(store, p.ID, "Obligation forgiven.")
	case "buy_grain":
		amount := clampInt(in.Amount, 1, marketMaxTrade)
		if amount <= 0 {
			setToastLocked(store, p.ID, "Choose a valid amount to buy.")
			return
		}
		base := marketBasePrice(store.World.GrainTier)
		buyPrice := marketBuyPrice(base, store.Policies.TaxRatePct, store.World.RestrictedMarketsTicks)
		supplySacks := store.World.GrainSupply / grainUnitPerSack
		if supplySacks <= 0 {
			setToastLocked(store, p.ID, "Market stalls are empty.")
			return
		}
		if amount > supplySacks {
			setToastLocked(store, p.ID, fmt.Sprintf("Market can only supply %d sacks.", supplySacks))
			return
		}
		totalCost := amount * buyPrice
		if p.Gold < totalCost {
			setToastLocked(store, p.ID, fmt.Sprintf("Need %dg to buy %d sacks.", totalCost, amount))
			return
		}
		p.Gold -= totalCost
		p.Grain += amount
		applyGrainSupplyDeltaLocked(store, now, -amount*grainUnitPerSack)
		addEventLocked(store, Event{Type: "Market", Severity: 1, Text: fmt.Sprintf("[%s] buys %d sacks from the market.", p.Name, amount), At: now})
		setToastLocked(store, p.ID, fmt.Sprintf("Bought %d sacks for %dg.", amount, totalCost))
	case "sell_grain":
		amount := clampInt(in.Amount, 1, marketMaxTrade)
		if amount <= 0 {
			setToastLocked(store, p.ID, "Choose a valid amount to sell.")
			return
		}
		if p.Grain < amount {
			setToastLocked(store, p.ID, fmt.Sprintf("You only hold %d sacks.", p.Grain))
			return
		}
		base := marketBasePrice(store.World.GrainTier)
		sellPrice := marketSellPrice(base, store.Policies.TaxRatePct, store.World.RestrictedMarketsTicks)
		totalGain := amount * sellPrice
		p.Grain -= amount
		p.Gold += totalGain
		applyGrainSupplyDeltaLocked(store, now, amount*grainUnitPerSack)
		addEventLocked(store, Event{Type: "Market", Severity: 1, Text: fmt.Sprintf("[%s] sells %d sacks into the market.", p.Name, amount), At: now})
		setToastLocked(store, p.ID, fmt.Sprintf("Sold %d sacks for %dg.", amount, totalGain))
	case "donate_relief":
		if p.Grain < reliefSackCost {
			setToastLocked(store, p.ID, fmt.Sprintf("Need %d sacks to fund relief.", reliefSackCost))
			return
		}
		p.Grain -= reliefSackCost
		applyGrainSupplyDeltaLocked(store, now, reliefSackCost*grainUnitPerSack)
		prevUnrest := store.World.UnrestTier
		store.World.UnrestValue = clampInt(store.World.UnrestValue-6, 0, 100)
		store.World.UnrestTier = unrestTierFromValue(store.World.UnrestValue)
		if store.World.UnrestTier != prevUnrest {
			addEventLocked(store, Event{Type: "Unrest", Severity: 2, Text: unrestTierNarrative(prevUnrest, store.World.UnrestTier), At: now})
		}
		p.Rep = clampInt(p.Rep+2, -100, 100)
		addEventLocked(store, Event{Type: "Relief", Severity: 2, Text: fmt.Sprintf("[%s] funds relief wagons for the hungry.", p.Name), At: now})
		setToastLocked(store, p.ID, "Relief funded; unrest eases.")
	case "bribe_official":
		targetSeat := store.Seats["harbor_master"]
		cost := maxInt(2, in.Amount)
		if p.Gold < cost {
			setToastLocked(store, p.ID, "You cannot afford the bribe.")
			return
		}
		p.Gold -= cost
		p.Heat = clampInt(p.Heat+2, 0, 20)
		duration := bribeAccessBaseTicks
		if cost >= 6 {
			duration++
		}
		p.BribeAccessTicks = minInt(bribeAccessMaxTicks, p.BribeAccessTicks+duration)
		if targetSeat != nil && targetSeat.HolderPlayerID != "" && targetSeat.HolderPlayerID != p.ID {
			p.Rep = clampInt(p.Rep+1, -100, 100)
		}
		addEventLocked(store, Event{Type: "Institution", Severity: 3, Text: fmt.Sprintf("[%s] bribes officials for temporary access.", p.Name), At: now})
		setToastLocked(store, p.ID, fmt.Sprintf("Bribe executed: access secured for %d ticks.", p.BribeAccessTicks))
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
		ev := strongestEvidenceForLocked(store, p.ID, target.ID)
		if ev == nil {
			setToastLocked(store, p.ID, "You need evidence before threatening exposure.")
			return
		}
		if !consumeHighImpactBudgetLocked(store, p.ID, now) {
			setToastLocked(store, p.ID, "Daily cap reached for high-impact actions.")
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
	case "travel":
		if p.TravelTicksLeft > 0 {
			setToastLocked(store, p.ID, "You are already on the road.")
			return
		}
		targetID := strings.TrimSpace(in.LocationID)
		if targetID == "" {
			setToastLocked(store, p.ID, "Choose a destination.")
			return
		}
		if targetID == p.LocationID {
			setToastLocked(store, p.ID, "You are already there.")
			return
		}
		if _, ok := locationByID(targetID); !ok {
			setToastLocked(store, p.ID, "Unknown destination.")
			return
		}
		ticks := travelTicksBetween(p.LocationID, targetID)
		if ticks <= 0 {
			setToastLocked(store, p.ID, "No travel needed.")
			return
		}
		p.TravelToID = targetID
		p.TravelTicksLeft = ticks
		p.TravelTotalTicks = ticks
		addEventLocked(store, Event{
			Type:     "Travel",
			Severity: 1,
			Text:     fmt.Sprintf("[%s] departs for %s.", p.Name, locationName(targetID)),
			At:       now,
		})
		setToastLocked(store, p.ID, fmt.Sprintf("You depart for %s (%dt).", locationName(targetID), ticks))
	case "scavenge_frontier":
		if p.LocationID != locationFrontier {
			setToastLocked(store, p.ID, "Travel to the Frontier Village to scavenge.")
			return
		}
		if remaining := fieldworkCooldownRemaining(store, p.ID); remaining > 0 {
			setToastLocked(store, p.ID, fmt.Sprintf("Fieldwork cooldown: %dt.", remaining))
			return
		}
		if p.Gold < fieldworkSupplyCost {
			setToastLocked(store, p.ID, fmt.Sprintf("Need %dg for supplies.", fieldworkSupplyCost))
			return
		}
		p.Gold -= fieldworkSupplyCost
		store.LastFieldworkAt[p.ID] = store.TickCount
		roll := store.rng.Intn(100)
		switch {
		case roll < 60:
			p.Grain += 2
			addEventLocked(store, Event{Type: "Fieldwork", Severity: 1, Text: fmt.Sprintf("[%s] scavenges 2 sacks from the frontier.", p.Name), At: now})
			setToastLocked(store, p.ID, "You return with 2 sacks.")
		case roll < 85:
			p.Gold += 3
			addEventLocked(store, Event{Type: "Fieldwork", Severity: 1, Text: fmt.Sprintf("[%s] sells salvaged supplies in the frontier.", p.Name), At: now})
			setToastLocked(store, p.ID, "You barter for 3g.")
		default:
			p.Heat = clampInt(p.Heat+1, 0, 20)
			p.Rep = clampInt(p.Rep-1, -100, 100)
			addEventLocked(store, Event{Type: "Fieldwork", Severity: 2, Text: fmt.Sprintf("[%s] returns from the frontier under suspicion.", p.Name), At: now})
			setToastLocked(store, p.ID, "Watch patrols notice your movements.")
		}
	case "explore_ruins":
		if p.LocationID != locationRuins {
			setToastLocked(store, p.ID, "Travel to the Haunted Ruins to explore.")
			return
		}
		if remaining := fieldworkCooldownRemaining(store, p.ID); remaining > 0 {
			setToastLocked(store, p.ID, fmt.Sprintf("Fieldwork cooldown: %dt.", remaining))
			return
		}
		if p.Gold < fieldworkSupplyCost {
			setToastLocked(store, p.ID, fmt.Sprintf("Need %dg for supplies.", fieldworkSupplyCost))
			return
		}
		p.Gold -= fieldworkSupplyCost
		store.LastFieldworkAt[p.ID] = store.TickCount
		roll := store.rng.Intn(100)
		switch {
		case roll < 45:
			p.Rep = clampInt(p.Rep+1, -100, 100)
			relic := addRelicLocked(store, p, randomRelicDefinition(store.rng), now)
			name := "a sealed relic"
			if relic != nil {
				name = relic.Name
			}
			setToastLocked(store, p.ID, fmt.Sprintf("Relic recovered: %s. Seek appraisal in the Capital.", name))
		case roll < 75:
			p.Rumors += 1
			addEventLocked(store, Event{Type: "Fieldwork", Severity: 1, Text: fmt.Sprintf("[%s] unearths whispers in the ruins.", p.Name), At: now})
			setToastLocked(store, p.ID, "Ruins rumors spread: +1 rumor.")
		default:
			p.Heat = clampInt(p.Heat+2, 0, 20)
			p.Rep = clampInt(p.Rep-2, -100, 100)
			addEventLocked(store, Event{Type: "Fieldwork", Severity: 3, Text: fmt.Sprintf("[%s] flees a hostile presence in the ruins.", p.Name), At: now})
			setToastLocked(store, p.ID, "You escape, but the city hears of it.")
		}
	case "appraise_relic":
		if in.RelicID == "" {
			setToastLocked(store, p.ID, "Choose a relic to appraise.")
			return
		}
		relicID, err := strconv.ParseInt(in.RelicID, 10, 64)
		if err != nil {
			setToastLocked(store, p.ID, "Relic entry invalid.")
			return
		}
		relic := store.Relics[relicID]
		if relic == nil || relic.OwnerPlayerID != p.ID {
			setToastLocked(store, p.ID, "Relic not found.")
			return
		}
		if relic.Status != relicStatusUnappraised {
			setToastLocked(store, p.ID, "Relic already appraised.")
			return
		}
		if p.LocationID != locationCapital {
			setToastLocked(store, p.ID, "Appraisals are only performed in the Capital.")
			return
		}
		if p.Gold < relicAppraiseCost {
			setToastLocked(store, p.ID, fmt.Sprintf("Need %dg to appraise.", relicAppraiseCost))
			return
		}
		p.Gold -= relicAppraiseCost
		relic.Status = relicStatusAppraised
		relic.AppraisedAtTick = store.TickCount
		addEventLocked(store, Event{
			Type:     "Temple",
			Severity: 2,
			Text:     fmt.Sprintf("[%s] appraises a relic: %s.", p.Name, relic.Name),
			At:       now,
		})
		setToastLocked(store, p.ID, fmt.Sprintf("Relic appraised: %s.", relicEffectLabel(relic)))
	case "invoke_relic":
		if in.RelicID == "" {
			setToastLocked(store, p.ID, "Choose a relic to invoke.")
			return
		}
		relicID, err := strconv.ParseInt(in.RelicID, 10, 64)
		if err != nil {
			setToastLocked(store, p.ID, "Relic entry invalid.")
			return
		}
		relic := store.Relics[relicID]
		if relic == nil || relic.OwnerPlayerID != p.ID {
			setToastLocked(store, p.ID, "Relic not found.")
			return
		}
		if relic.Status != relicStatusAppraised {
			setToastLocked(store, p.ID, "Relic must be appraised first.")
			return
		}
		switch relic.Effect {
		case "heat":
			p.Heat = maxInt(0, p.Heat-relic.Power)
		case "rep":
			p.Rep = clampInt(p.Rep+relic.Power, -100, 100)
		case "gold":
			p.Gold += relic.Power
		case "rumor":
			p.Rumors += relic.Power
		case "grain":
			p.Grain += relic.Power
		}
		addEventLocked(store, Event{
			Type:     "Relic",
			Severity: 2,
			Text:     fmt.Sprintf("[%s] invokes %s.", p.Name, relic.Name),
			At:       now,
		})
		delete(store.Relics, relic.ID)
		setToastLocked(store, p.ID, fmt.Sprintf("Relic invoked: %s.", relicEffectLabel(relic)))
	case "launch_project":
		def, ok := projectDefinitionByType(in.ProjectType)
		if !ok {
			setToastLocked(store, p.ID, "Project type not found.")
			return
		}
		if len(store.Projects) >= projectMaxActive {
			setToastLocked(store, p.ID, "City project capacity reached.")
			return
		}
		if playerHasActiveProjectLocked(store, p.ID) {
			setToastLocked(store, p.ID, "You already have a project underway.")
			return
		}
		if !consumeHighImpactBudgetLocked(store, p.ID, now) {
			setToastLocked(store, p.ID, "Daily cap reached for high-impact actions.")
			return
		}
		if p.Gold < def.CostGold {
			setToastLocked(store, p.ID, fmt.Sprintf("Need %dg to fund this project.", def.CostGold))
			return
		}
		if p.Grain < def.CostGrain {
			setToastLocked(store, p.ID, fmt.Sprintf("Need %d sacks to fund this project.", def.CostGrain))
			return
		}
		p.Gold -= def.CostGold
		if def.CostGrain > 0 {
			p.Grain -= def.CostGrain
		}
		store.NextProjectID++
		id := fmt.Sprintf("p-%d", store.NextProjectID)
		store.Projects[id] = &Project{
			ID:            id,
			Type:          def.Type,
			Name:          def.Name,
			OwnerPlayerID: p.ID,
			OwnerName:     p.Name,
			CostGold:      def.CostGold,
			CostGrain:     def.CostGrain,
			TicksLeft:     def.DurationTicks,
			TotalTicks:    def.DurationTicks,
		}
		addEventLocked(store, Event{
			Type:     "Civic",
			Severity: 2,
			Text:     fmt.Sprintf("[%s] funds %s (%d ticks).", p.Name, def.Name, def.DurationTicks),
			At:       now,
		})
		setToastLocked(store, p.ID, fmt.Sprintf("%s funded.", def.Name))
	case "respond_crisis":
		if store.ActiveCrisis == nil {
			setToastLocked(store, p.ID, "No active crisis to address.")
			return
		}
		def, ok := crisisDefinitionByType(store.ActiveCrisis.Type)
		if !ok {
			setToastLocked(store, p.ID, "Crisis details unavailable.")
			return
		}
		if !consumeHighImpactBudgetLocked(store, p.ID, now) {
			setToastLocked(store, p.ID, "Daily cap reached for high-impact actions.")
			return
		}
		if p.Gold < def.GoldCost {
			setToastLocked(store, p.ID, fmt.Sprintf("Need %dg to mobilize a response.", def.GoldCost))
			return
		}
		if def.GrainCost > 0 && p.Grain < def.GrainCost {
			setToastLocked(store, p.ID, fmt.Sprintf("Need %d sacks to mobilize a response.", def.GrainCost))
			return
		}
		p.Gold -= def.GoldCost
		if def.GrainCost > 0 {
			p.Grain -= def.GrainCost
		}
		store.ActiveCrisis.Mitigated = true
		if store.ActiveCrisis.Severity > 1 {
			store.ActiveCrisis.Severity--
		}
		store.ActiveCrisis.TicksLeft = maxInt(0, store.ActiveCrisis.TicksLeft-1)
		if def.ResolveRepDelta != 0 {
			p.Rep = clampInt(p.Rep+def.ResolveRepDelta, -100, 100)
		}
		if def.ResolveUnrestDelta != 0 {
			store.World.UnrestValue = clampInt(store.World.UnrestValue-def.ResolveUnrestDelta, 0, 100)
			store.World.UnrestTier = unrestTierFromValue(store.World.UnrestValue)
		}
		addEventLocked(store, Event{
			Type:     "Crisis",
			Severity: 2,
			Text:     fmt.Sprintf("[%s] mobilizes a response to %s.", p.Name, def.Name),
			At:       now,
		})
		setToastLocked(store, p.ID, "Response deployed.")
		if store.ActiveCrisis.TicksLeft <= 0 {
			resolveCrisisLocked(store, def, now, true, p)
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
	case "conduct_inquest":
		if !playerHoldsSeatLocked(store, p.ID, "high_curate") {
			setToastLocked(store, p.ID, "Only the High Curate can conduct inquests.")
			return
		}
		target := store.Players[in.TargetID]
		if target == nil || target.ID == p.ID {
			setToastLocked(store, p.ID, "Choose a valid inquest target.")
			return
		}
		if !consumeHighImpactBudgetLocked(store, p.ID, now) {
			setToastLocked(store, p.ID, "Daily cap reached for high-impact actions.")
			return
		}
		removedEvidence := 0
		for id, ev := range store.Evidence {
			if ev.TargetPlayerID == target.ID && ev.Forged {
				delete(store.Evidence, id)
				removedEvidence++
			}
		}
		rumorAdjusted := 0
		for _, r := range store.Rumors {
			if r.TargetPlayerID != target.ID {
				continue
			}
			r.Credibility = maxInt(1, r.Credibility-inquestRumorCredibilityDrop)
			r.Spread = maxInt(0, r.Spread-inquestRumorSpreadDrop)
			r.Decay = maxInt(0, r.Decay-inquestRumorDecayDrop)
			rumorAdjusted++
		}
		addEventLocked(store, Event{
			Type:     "Doctrine",
			Severity: 2,
			Text:     fmt.Sprintf("[%s] conducts an inquest over [%s].", p.Name, target.Name),
			At:       now,
		})
		if removedEvidence == 0 && rumorAdjusted == 0 {
			setToastLocked(store, p.ID, "Inquest finds no false testimony to purge.")
			return
		}
		target.Rep = clampInt(target.Rep+1, -100, 100)
		target.Heat = maxInt(0, target.Heat-1)
		p.Rep = clampInt(p.Rep+1, -100, 100)
		setToastLocked(store, p.ID, fmt.Sprintf("Inquest purges %d forged dossiers and dampens %d rumor lines.", removedEvidence, rumorAdjusted))
		setToastLocked(store, target.ID, "An inquest clears your name with the Curate.")
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
	case "issue_permit":
		if !playerHoldsSeatLocked(store, p.ID, "harbor_master") {
			setToastLocked(store, p.ID, "Only the Harbor Master can issue permits.")
			return
		}
		if !store.Policies.PermitRequiredHighRisk {
			setToastLocked(store, p.ID, "Permits are currently open; no permit needed.")
			return
		}
		target := store.Players[in.TargetID]
		if target == nil {
			setToastLocked(store, p.ID, "Select a valid permit recipient.")
			return
		}
		if hasActivePermitLocked(store, target.ID) {
			setToastLocked(store, p.ID, "That player already holds a permit.")
			return
		}
		if !consumeHighImpactBudgetLocked(store, p.ID, now) {
			setToastLocked(store, p.ID, "Daily cap reached for high-impact actions.")
			return
		}
		store.Permits[target.ID] = &Permit{
			PlayerID:     target.ID,
			PlayerName:   target.Name,
			IssuerID:     p.ID,
			IssuerName:   p.Name,
			TicksLeft:    permitDurationTicks,
			TotalTicks:   permitDurationTicks,
			IssuedAtTick: store.TickCount,
		}
		addEventLocked(store, Event{Type: "Policy", Severity: 2, Text: fmt.Sprintf("[%s] issues a permit to [%s].", p.Name, target.Name), At: now})
		setToastLocked(store, p.ID, "Permit issued.")
	case "issue_warrant":
		if !playerHoldsSeatLocked(store, p.ID, "watch_commander") {
			setToastLocked(store, p.ID, "Only the Commander of the Watch can issue warrants.")
			return
		}
		target := store.Players[in.TargetID]
		if target == nil || target.ID == p.ID {
			setToastLocked(store, p.ID, "Choose a valid warrant target.")
			return
		}
		if hasActiveWarrantLocked(store, target.ID) {
			setToastLocked(store, p.ID, "That target is already under warrant.")
			return
		}
		if !consumeHighImpactBudgetLocked(store, p.ID, now) {
			setToastLocked(store, p.ID, "Daily cap reached for high-impact actions.")
			return
		}
		store.Warrants[target.ID] = &Warrant{
			PlayerID:     target.ID,
			PlayerName:   target.Name,
			IssuerID:     p.ID,
			IssuerName:   p.Name,
			TicksLeft:    warrantDurationTicks,
			TotalTicks:   warrantDurationTicks,
			IssuedAtTick: store.TickCount,
		}
		target.Heat = clampInt(target.Heat+warrantHeatDelta, 0, 20)
		target.Rep = clampInt(target.Rep-1, -100, 100)
		addEventLocked(store, Event{Type: "Law", Severity: 3, Text: fmt.Sprintf("[%s] issues a warrant on [%s].", p.Name, target.Name), At: now})
		if !hasActiveBountyForTargetLocked(store, target.ID) {
			issueBountyContractLocked(store, target, bountyDeadlineTicks)
			addEventLocked(store, Event{Type: "Law", Severity: 3, Text: fmt.Sprintf("[City Watch] posts a warrant bounty on [%s].", target.Name), At: now})
		}
		setToastLocked(store, p.ID, fmt.Sprintf("Warrant issued for %s.", target.Name))
		setToastLocked(store, target.ID, "A warrant has been issued in your name.")
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
	note := stanceEventText(outcome.Stance)
	if c.Type == "Bounty" {
		note = "Your report tightens the city's grip on crime."
	}
	addEventLocked(store, Event{Type: "Consequence", Severity: 1, Text: note, At: now})
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
		if parsed, ok := parsePlayerCookieValue(c.Value); ok {
			pid = parsed
		}
	}
	if pid == "" {
		pid = generateID()
		http.SetCookie(w, &http.Cookie{
			Name:     cookieName,
			Value:    playerCookieValue(pid),
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			Secure:   envBool(playerCookieSecureEnvName, requestIsHTTPS(r)),
		})
	}

	p := store.Players[pid]
	if p == nil {
		p = &Player{ID: pid, Name: uniqueGuestNameLocked(store), Gold: initialPlayerGold, Grain: 0, Rep: 0, LastSeen: time.Now().UTC()}
		store.Players[pid] = p
		setToastLocked(store, pid, fmt.Sprintf("You arrive as %s.", p.Name))
		addEventLocked(store, Event{Type: "Join", Severity: 1, Text: fmt.Sprintf("[%s] enters the city under a borrowed name.", p.Name), At: time.Now().UTC()})
	}
	p.SoftDeletedAt = time.Time{}
	p.HardDeletedAt = time.Time{}
	if p.LocationID == "" {
		p.LocationID = locationCapital
	}
	return p
}

func requestIsHTTPS(r *http.Request) bool {
	if r == nil {
		return false
	}
	if r.TLS != nil {
		return true
	}
	xfp := strings.TrimSpace(strings.ToLower(r.Header.Get("X-Forwarded-Proto")))
	return xfp == "https"
}

func playerCookieValue(playerID string) string {
	return playerID + "." + signPlayerCookieValue(playerID)
}

func parsePlayerCookieValue(raw string) (string, bool) {
	parts := strings.Split(raw, ".")
	if len(parts) != 2 {
		return "", false
	}
	playerID := strings.TrimSpace(parts[0])
	signature := strings.TrimSpace(parts[1])
	if playerID == "" || signature == "" {
		return "", false
	}
	if !secureStringEqual(signature, signPlayerCookieValue(playerID)) {
		return "", false
	}
	return playerID, true
}

func signPlayerCookieValue(playerID string) string {
	h := hmac.New(sha256.New, getPlayerCookieSecret())
	_, _ = io.WriteString(h, "black-granary-player-cookie-v1:")
	_, _ = io.WriteString(h, playerID)
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

func getPlayerCookieSecret() []byte {
	playerCookieSecretOnce.Do(func() {
		secret := strings.TrimSpace(os.Getenv(playerCookieSecretEnvName))
		if secret != "" {
			playerCookieSecret = []byte(secret)
			return
		}
		generated, err := generateIDFromRandomBytes(32)
		if err != nil {
			playerCookieSecret = []byte(fmt.Sprintf("fallback-%d", time.Now().UnixNano()))
			return
		}
		playerCookieSecret = []byte(generated)
	})
	return playerCookieSecret
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

func addDiplomacyMessageLocked(store *Store, msg DiplomaticMessage) {
	store.NextMessageID++
	msg.ID = store.NextMessageID
	if msg.At.IsZero() {
		msg.At = time.Now().UTC()
	}
	store.Messages = append(store.Messages, msg)
	if len(store.Messages) > maxDiplomacyMessages {
		store.Messages = store.Messages[len(store.Messages)-maxDiplomacyMessages:]
	}
}

func issueContractLocked(store *Store, ctype string, deadline int) {
	store.NextContractID++
	id := fmt.Sprintf("c-%d", store.NextContractID)
	store.Contracts[id] = &Contract{ID: id, Type: ctype, DeadlineTicks: deadline, Status: "Issued", IssuedAtTick: store.TickCount}
}

func issueBountyContractLocked(store *Store, target *Player, deadline int) {
	if store == nil || target == nil {
		return
	}
	store.NextContractID++
	id := fmt.Sprintf("c-%d", store.NextContractID)
	reward := clampInt(18+target.Heat*2, 20, 70)
	warranted := false
	if hasActiveWarrantLocked(store, target.ID) {
		reward += warrantRewardBonus
		warranted = true
	}
	store.Contracts[id] = &Contract{
		ID:             id,
		Type:           "Bounty",
		DeadlineTicks:  deadline,
		Status:         "Issued",
		IssuedAtTick:   store.TickCount,
		TargetPlayerID: target.ID,
		TargetName:     target.Name,
		BountyReward:   reward,
		BountyEvidence: bountyEvidenceMin,
		Warranted:      warranted,
	}
}

func issueSupplyContractLocked(store *Store, issuer *Player, sacks, reward, deadline int) *Contract {
	if store == nil || issuer == nil {
		return nil
	}
	store.NextContractID++
	id := fmt.Sprintf("c-%d", store.NextContractID)
	c := &Contract{
		ID:             id,
		Type:           "Supply",
		DeadlineTicks:  deadline,
		Status:         "Issued",
		IssuedAtTick:   store.TickCount,
		IssuerPlayerID: issuer.ID,
		IssuerName:     issuer.Name,
		RewardGold:     reward,
		SupplySacks:    sacks,
	}
	store.Contracts[id] = c
	return c
}

func hasActiveSupplyFromIssuerLocked(store *Store, issuerID string) bool {
	for _, c := range store.Contracts {
		if c.Type != "Supply" || c.IssuerPlayerID != issuerID {
			continue
		}
		switch c.Status {
		case "Issued", "Accepted", "Fulfilled":
			return true
		}
	}
	return false
}

func hasActiveContractLocked(store *Store, ctype string) bool {
	for _, c := range store.Contracts {
		if c.Type == ctype && (c.Status == "Issued" || c.Status == "Accepted") {
			return true
		}
	}
	return false
}

func hasActiveBountyForTargetLocked(store *Store, targetID string) bool {
	for _, c := range store.Contracts {
		if c.Type != "Bounty" || c.TargetPlayerID != targetID {
			continue
		}
		switch c.Status {
		case "Issued", "Accepted", "Fulfilled":
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

func playerHasActiveProjectLocked(store *Store, playerID string) bool {
	for _, proj := range store.Projects {
		if proj.OwnerPlayerID == playerID {
			return true
		}
	}
	return false
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
	highImpactRemaining := highImpactDailyCap
	if store.DailyActionDate[playerID] == today {
		highImpactRemaining = highImpactDailyCap - store.DailyHighImpactN[playerID]
	}
	if highImpactRemaining < 0 {
		highImpactRemaining = 0
	}

	contractView := func(c *Contract) ContractView {
		isBounty := c.Type == "Bounty"
		isSupply := c.Type == "Supply"
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
		issuerName := ""
		if c.IssuerPlayerID != "" {
			issuerName = c.IssuerName
			if issuerP := store.Players[c.IssuerPlayerID]; issuerP != nil {
				issuerName = fmt.Sprintf("%s (%s)", issuerP.Name, reputationTitle(issuerP.Rep))
			}
		}
		canAccept := c.Status == "Issued"
		canIgnore := c.Status == "Issued"
		if isBounty && c.TargetPlayerID == p.ID {
			canAccept = false
			canIgnore = false
		}
		if c.IssuerPlayerID != "" && c.IssuerPlayerID == p.ID {
			canAccept = false
			canIgnore = false
		}
		canAbandon := c.Status == "Accepted" && c.OwnerPlayerID == p.ID
		canCancel := c.Status == "Issued" && c.IssuerPlayerID == p.ID
		canDeliver := (c.Status == "Accepted" && c.OwnerPlayerID == p.ID) || (c.Status == "Fulfilled" && c.OwnerPlayerID == p.ID)
		hasBribedAccess := p.BribeAccessTicks > 0
		permitRequired := c.Type == "Emergency" && store.Policies.PermitRequiredHighRisk && p.Rep < 20 && !playerHoldsSeatLocked(store, p.ID, "harbor_master") && !hasActivePermitLocked(store, p.ID) && !hasBribedAccess
		embargoBlocks := c.Type == "Smuggling" && store.Policies.SmugglingEmbargoTicks > 0 && !playerHoldsSeatLocked(store, p.ID, "harbor_master") && !hasBribedAccess
		if permitRequired || embargoBlocks {
			canAccept = false
		}

		showOutcome := c.OwnerPlayerID == p.ID && (c.Status == "Accepted" || c.Status == "Fulfilled")
		deliverDisabled := false
		outcomeLabel := ""
		outcomeNote := ""
		requirementNote := ""
		rewardNote := ""
		var outcome DeliverOutcome
		if showOutcome {
			outcome = computeDeliverOutcomeLocked(store, p, c)
			outcomeLabel = fmt.Sprintf("%+dg, %+d rep, %+d heat", outcome.RewardGold, outcome.RepDelta, outcome.HeatDelta)
			if c.Status == "Accepted" && !isBounty && !isSupply {
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
			if !isBounty && !isSupply && p.Rumors > 0 {
				if outcomeNote != "" {
					outcomeNote += " "
				}
				outcomeNote += "Rumor bonus ready."
			}
			if isBounty {
				required := c.BountyEvidence
				if required <= 0 {
					required = bountyEvidenceMin
				}
				ev := strongestEvidenceForLocked(store, p.ID, c.TargetPlayerID)
				if ev == nil || ev.Strength < required {
					deliverDisabled = true
					outcomeNote = fmt.Sprintf("Need evidence strength %d+ on target.", required)
				}
			}
			if isSupply && c.SupplySacks > 0 && p.Grain < c.SupplySacks {
				deliverDisabled = true
				outcomeNote = fmt.Sprintf("Need %d sacks to deliver.", c.SupplySacks)
			}
		}
		requirementNotes := []string{}
		if permitRequired {
			requirementNotes = append(requirementNotes, "Requirement: permit required for emergency contracts.")
		}
		if embargoBlocks {
			requirementNotes = append(requirementNotes, fmt.Sprintf("Requirement: smuggling embargo active (%dt).", store.Policies.SmugglingEmbargoTicks))
		}
		if hasBribedAccess && c.Type == "Emergency" && store.Policies.PermitRequiredHighRisk && p.Rep < 20 && !playerHoldsSeatLocked(store, p.ID, "harbor_master") {
			requirementNotes = append(requirementNotes, fmt.Sprintf("Bribed access bypasses permits (%dt).", p.BribeAccessTicks))
		}
		if hasBribedAccess && c.Type == "Smuggling" && store.Policies.SmugglingEmbargoTicks > 0 && !playerHoldsSeatLocked(store, p.ID, "harbor_master") {
			requirementNotes = append(requirementNotes, fmt.Sprintf("Bribed access bypasses embargo (%dt).", p.BribeAccessTicks))
		}
		if isBounty {
			required := c.BountyEvidence
			if required <= 0 {
				required = bountyEvidenceMin
			}
			requirementNotes = append(requirementNotes, fmt.Sprintf("Requirement: evidence strength %d+ on target.", required))
			if c.BountyReward > 0 {
				if c.Warranted {
					rewardNote = fmt.Sprintf("Reward: %dg (warranted).", c.BountyReward)
				} else {
					rewardNote = fmt.Sprintf("Reward: %dg.", c.BountyReward)
				}
			}
		}
		if isSupply {
			if c.SupplySacks > 0 {
				requirementNotes = append(requirementNotes, fmt.Sprintf("Requirement: deliver %d sacks.", c.SupplySacks))
			}
			if c.RewardGold > 0 {
				rewardNote = fmt.Sprintf("Reward: %dg escrowed.", c.RewardGold)
			}
		}
		if len(requirementNotes) > 0 {
			requirementNote = strings.Join(requirementNotes, " ")
		}

		deliverLabel := "Deliver"
		if canDeliver && showOutcome {
			netGold := outcome.RewardGold
			if c.Status == "Accepted" && c.OwnerPlayerID == p.ID && !isBounty && !isSupply {
				netGold -= 2
			}
			deliverLabel = fmt.Sprintf("Deliver (%+dg)", netGold)
		}
		stanceValue := normalizeContractStance(c.Stance)
		if isBounty || isSupply {
			stanceValue = ""
		}
		iconPath, iconTint := contractTypeIcon(c.Type)
		return ContractView{
			ID:              c.ID,
			Type:            c.Type,
			Status:          c.Status,
			DeadlineTicks:   c.DeadlineTicks,
			OwnerName:       owner,
			IssuerName:      issuerName,
			Stance:          stanceValue,
			TargetName:      c.TargetName,
			UrgencyClass:    urgency,
			CanAccept:       canAccept,
			CanIgnore:       canIgnore,
			CanAbandon:      canAbandon,
			CanCancel:       canCancel,
			CanDeliver:      canDeliver,
			DeliverLabel:    deliverLabel,
			DeliverDisabled: deliverDisabled,
			ShowOutcome:     showOutcome,
			OutcomeLabel:    outcomeLabel,
			OutcomeNote:     outcomeNote,
			RequirementNote: requirementNote,
			RewardNote:      rewardNote,
			IsBounty:        isBounty,
			IsSupply:        isSupply,
			IconPath:        iconPath,
			IconTint:        iconTint,
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
		warrantLabel := ""
		if warrant := warrantForPlayerLocked(store, pl.ID); warrant != nil {
			warrantLabel = fmt.Sprintf("Warrant (%dt)", warrant.TicksLeft)
		}
		isOnline := now.Sub(pl.LastSeen) <= onlineWindow
		iconTint := "blue"
		if isOnline {
			iconTint = "lime"
		}
		players = append(players, PlayerSummary{
			Name:      pl.Name,
			Rep:       pl.Rep,
			Title:     reputationTitle(pl.Rep),
			Gold:      pl.Gold,
			Heat:      pl.Heat,
			HeatLabel: standingHeatLabel(pl.Heat),
			Warrant:   warrantLabel,
			Online:    isOnline,
			IconPath:  iconAsset("delapouite", "meeple-circle"),
			IconTint:  iconTint,
		})
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

	messages := []MessageView{}
	for _, m := range store.Messages {
		if m.FromPlayerID != p.ID && m.ToPlayerID != p.ID {
			continue
		}
		direction := "Received"
		if m.FromPlayerID == p.ID {
			direction = "Sent"
		}
		messages = append(messages, MessageView{
			FromName:  m.FromName,
			ToName:    m.ToName,
			Subject:   m.Subject,
			Body:      m.Body,
			Direction: direction,
			At:        m.At.Format("15:04:05"),
			Sealed:    m.Sealed,
		})
	}
	if len(messages) > maxVisibleMessages {
		messages = messages[len(messages)-maxVisibleMessages:]
	}

	playerOptions := make([]PlayerOption, 0, len(store.Players))
	for _, pp := range store.Players {
		if pp.ID == p.ID {
			continue
		}
		playerOptions = append(playerOptions, PlayerOption{ID: pp.ID, Name: pp.Name})
	}
	sort.Slice(playerOptions, func(i, j int) bool { return playerOptions[i].Name < playerOptions[j].Name })
	hasOtherPlayers := len(playerOptions) > 0
	sealMessageDisabled := false
	sealMessageNote := ""
	if p.Gold < sealedMessageCost {
		sealMessageDisabled = true
		sealMessageNote = fmt.Sprintf("Need %dg to seal a missive.", sealedMessageCost)
	}

	seatOrder := []string{"harbor_master", "master_of_coin", "watch_commander", "high_curate"}
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
		canIssuePermit := seat.ID == "harbor_master" && seat.HolderPlayerID == p.ID && store.Policies.PermitRequiredHighRisk && hasOtherPlayers && highImpactRemaining > 0
		canConductInquest := seat.ID == "high_curate" && seat.HolderPlayerID == p.ID && hasOtherPlayers && highImpactRemaining > 0
		canIssueWarrant := seat.ID == "watch_commander" && seat.HolderPlayerID == p.ID && hasOtherPlayers && highImpactRemaining > 0
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
			CanIssuePermit:      canIssuePermit,
			CanConductInquest:   canConductInquest,
			CanIssueWarrant:     canIssueWarrant,
		})
	}

	if p.LocationID == "" {
		p.LocationID = locationCapital
	}
	locationDef, _ := locationByID(p.LocationID)
	locationIconPath, locationIconTint := locationIcon(p.LocationID)
	traveling := p.TravelTicksLeft > 0
	travelDestination := ""
	if traveling {
		travelDestination = locationName(p.TravelToID)
	}
	forgeEvidenceDisabled := false
	forgeEvidenceReason := ""
	if traveling {
		forgeEvidenceDisabled = true
		forgeEvidenceReason = "Traveling."
	} else if highImpactRemaining == 0 {
		forgeEvidenceDisabled = true
		forgeEvidenceReason = "High-impact cap reached."
	} else if p.Gold < forgeEvidenceCost {
		forgeEvidenceDisabled = true
		forgeEvidenceReason = fmt.Sprintf("Need %dg to forge a dossier.", forgeEvidenceCost)
	}
	locationOptions := make([]LocationOption, 0, len(locationDefinitions()))
	for _, def := range locationDefinitions() {
		if def.ID == p.LocationID {
			continue
		}
		disabled := traveling
		reason := ""
		if disabled {
			reason = "Travel in progress."
		}
		optionIconPath, optionIconTint := locationIcon(def.ID)
		locationOptions = append(locationOptions, LocationOption{
			ID:          def.ID,
			Name:        def.Name,
			TravelTicks: travelTicksBetween(p.LocationID, def.ID),
			Disabled:    disabled,
			Reason:      reason,
			IconPath:    optionIconPath,
			IconTint:    optionIconTint,
		})
	}

	fieldworkAvailable := false
	fieldworkAction := ""
	fieldworkLabel := ""
	fieldworkDescription := ""
	fieldworkDisabled := false
	fieldworkDisabledReason := ""
	if traveling {
		fieldworkDisabled = true
		fieldworkDisabledReason = "Traveling."
	}
	switch p.LocationID {
	case locationFrontier:
		fieldworkAvailable = true
		fieldworkAction = "scavenge_frontier"
		fieldworkLabel = "Scavenge Hinterlands"
		fieldworkDescription = "Search the frontier for surplus sacks or salvage."
	case locationRuins:
		fieldworkAvailable = true
		fieldworkAction = "explore_ruins"
		fieldworkLabel = "Explore Ruins"
		fieldworkDescription = "Probe the ruins for relics, rumors, or danger."
	}
	if fieldworkAvailable && !fieldworkDisabled {
		if remaining := fieldworkCooldownRemaining(store, p.ID); remaining > 0 {
			fieldworkDisabled = true
			fieldworkDisabledReason = fmt.Sprintf("Fieldwork cooldown: %dt.", remaining)
		} else if p.Gold < fieldworkSupplyCost {
			fieldworkDisabled = true
			fieldworkDisabledReason = fmt.Sprintf("Need %dg for supplies.", fieldworkSupplyCost)
		}
	}

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
		sourceNote := "investigated"
		if ev.Forged {
			sourceNote = "forged"
		}
		evidence = append(evidence, EvidenceView{
			ID:         ev.ID,
			Topic:      ev.Topic,
			TargetName: ev.TargetName,
			SourceName: ev.SourceName,
			Strength:   ev.Strength,
			ExpiryIn:   int64(maxInt(0, int(ev.ExpiryTick-store.TickCount))),
			SourceNote: sourceNote,
		})
	}
	sort.Slice(evidence, func(i, j int) bool { return evidence[i].ID > evidence[j].ID })
	if len(evidence) > 8 {
		evidence = evidence[:8]
	}

	scryReports := make([]ScryReportView, 0, len(store.ScryReports))
	for _, report := range store.ScryReports {
		if report.OwnerPlayerID != p.ID {
			continue
		}
		locName := locationName(report.LocationID)
		travelNote := ""
		if report.TravelTicksLeft > 0 {
			travelNote = fmt.Sprintf("En route to %s (%dt left)", locationName(report.TravelToID), report.TravelTicksLeft)
		}
		scryReports = append(scryReports, ScryReportView{
			ID:           report.ID,
			TargetName:   report.TargetName,
			LocationName: locName,
			TravelNote:   travelNote,
			Rep:          report.Rep,
			Heat:         report.Heat,
			Gold:         report.Gold,
			Grain:        report.Grain,
			ExpiryIn:     int64(maxInt(0, int(report.ExpiryTick-store.TickCount))),
		})
	}
	sort.Slice(scryReports, func(i, j int) bool { return scryReports[i].ID > scryReports[j].ID })
	if len(scryReports) > 6 {
		scryReports = scryReports[:6]
	}

	intercepts := make([]InterceptView, 0, len(store.Intercepts))
	for _, intercept := range store.Intercepts {
		if intercept.OwnerPlayerID != p.ID {
			continue
		}
		subject := intercept.Subject
		body := intercept.Body
		if intercept.Sealed {
			subject = "Sealed missive"
			body = "Ciphered script resists your agents."
		}
		intercepts = append(intercepts, InterceptView{
			ID:       intercept.ID,
			FromName: intercept.FromName,
			ToName:   intercept.ToName,
			Subject:  subject,
			Body:     body,
			At:       intercept.At.Format("15:04:05"),
			ExpiryIn: int64(maxInt(0, int(intercept.ExpiryTick-store.TickCount))),
			Sealed:   intercept.Sealed,
		})
	}
	sort.Slice(intercepts, func(i, j int) bool { return intercepts[i].ID > intercepts[j].ID })
	if len(intercepts) > maxVisibleIntercepts {
		intercepts = intercepts[:maxVisibleIntercepts]
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
		cost := obligationCost(ob.Severity)
		canSettle := ob.DebtorPlayerID == p.ID && (ob.Status == "Open" || ob.Status == "Overdue")
		settleLabel := fmt.Sprintf("Settle (%dg)", cost)
		settleDisabled := false
		if canSettle && p.Gold < cost {
			settleDisabled = true
			settleLabel = fmt.Sprintf("Need %dg", cost)
		}
		canForgive := ob.CreditorPlayerID == p.ID && (ob.Status == "Open" || ob.Status == "Overdue")
		obligations = append(obligations, ObligationView{
			ID:             ob.ID,
			CreditorName:   ob.CreditorName,
			DebtorName:     ob.DebtorName,
			Reason:         ob.Reason,
			Severity:       ob.Severity,
			DueIn:          int64(maxInt(0, int(ob.DueTick-store.TickCount))),
			Status:         ob.Status,
			Cost:           cost,
			CanSettle:      canSettle,
			SettleLabel:    settleLabel,
			SettleDisabled: settleDisabled,
			CanForgive:     canForgive,
		})
	}
	sort.Slice(obligations, func(i, j int) bool { return obligations[i].ID > obligations[j].ID })

	permits := make([]PermitView, 0, len(store.Permits))
	for _, permit := range store.Permits {
		if permit == nil || permit.TicksLeft <= 0 {
			continue
		}
		permits = append(permits, PermitView{
			PlayerName: permit.PlayerName,
			IssuerName: permit.IssuerName,
			TicksLeft:  permit.TicksLeft,
		})
	}
	sort.Slice(permits, func(i, j int) bool { return permits[i].PlayerName < permits[j].PlayerName })

	warrants := make([]WarrantView, 0, len(store.Warrants))
	for _, warrant := range store.Warrants {
		if warrant == nil || warrant.TicksLeft <= 0 {
			continue
		}
		warrants = append(warrants, WarrantView{
			PlayerName: warrant.PlayerName,
			IssuerName: warrant.IssuerName,
			TicksLeft:  warrant.TicksLeft,
		})
	}
	sort.Slice(warrants, func(i, j int) bool { return warrants[i].PlayerName < warrants[j].PlayerName })

	relics := make([]RelicView, 0, len(store.Relics))
	for _, relic := range store.Relics {
		if relic == nil || relic.OwnerPlayerID != p.ID {
			continue
		}
		effectLabel := "Effect: unknown"
		if relic.Status == relicStatusAppraised {
			effectLabel = relicEffectLabel(relic)
		}
		canAppraise := relic.Status == relicStatusUnappraised
		appraiseDisabled := false
		appraiseReason := ""
		if canAppraise {
			if traveling {
				appraiseDisabled = true
				appraiseReason = "Traveling."
			} else if p.LocationID != locationCapital {
				appraiseDisabled = true
				appraiseReason = "Appraisals only in the Capital."
			} else if p.Gold < relicAppraiseCost {
				appraiseDisabled = true
				appraiseReason = fmt.Sprintf("Need %dg.", relicAppraiseCost)
			}
		}
		canInvoke := relic.Status == relicStatusAppraised
		invokeDisabled := false
		invokeReason := ""
		if canInvoke && traveling {
			invokeDisabled = true
			invokeReason = "Traveling."
		}
		relics = append(relics, RelicView{
			ID:                     relic.ID,
			Name:                   relic.Name,
			Status:                 relic.Status,
			EffectLabel:            effectLabel,
			CanAppraise:            canAppraise,
			AppraiseDisabled:       appraiseDisabled,
			AppraiseDisabledReason: appraiseReason,
			CanInvoke:              canInvoke,
			InvokeDisabled:         invokeDisabled,
			InvokeDisabledReason:   invokeReason,
		})
	}
	sort.Slice(relics, func(i, j int) bool { return relics[i].ID > relics[j].ID })
	if len(relics) > relicMaxVisible {
		relics = relics[:relicMaxVisible]
	}

	permitStatus := "None"
	if permit := permitForPlayerLocked(store, p.ID); permit != nil {
		permitStatus = fmt.Sprintf("Active (%dt)", permit.TicksLeft)
	}
	accessStatus := "Clear"
	if p.BribeAccessTicks > 0 {
		accessStatus = fmt.Sprintf("Bribed (%dt)", p.BribeAccessTicks)
	}
	warrantStatus := "Clear"
	if warrant := warrantForPlayerLocked(store, p.ID); warrant != nil {
		warrantStatus = fmt.Sprintf("Active (%dt)", warrant.TicksLeft)
	}

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

	projects := make([]ProjectView, 0, len(store.Projects))
	for _, proj := range store.Projects {
		owner := proj.OwnerName
		if owner == "" {
			owner = "-"
		} else if ownerP := store.Players[proj.OwnerPlayerID]; ownerP != nil {
			owner = fmt.Sprintf("%s (%s)", ownerP.Name, reputationTitle(ownerP.Rep))
		}
		effectNote := "effects pending"
		if def, ok := projectDefinitionByType(proj.Type); ok {
			effectNote = projectEffectNote(def)
		}
		projects = append(projects, ProjectView{
			ID:         proj.ID,
			Name:       proj.Name,
			OwnerName:  owner,
			TicksLeft:  proj.TicksLeft,
			EffectNote: effectNote,
		})
	}
	sort.Slice(projects, func(i, j int) bool {
		if projects[i].TicksLeft != projects[j].TicksLeft {
			return projects[i].TicksLeft < projects[j].TicksLeft
		}
		return projects[i].Name < projects[j].Name
	})

	projectOptions := make([]ProjectOption, 0, len(projectDefinitions()))
	playerHasProject := playerHasActiveProjectLocked(store, p.ID)
	activeProjectCount := len(store.Projects)
	for _, def := range projectDefinitions() {
		disabledReason := ""
		if activeProjectCount >= projectMaxActive {
			disabledReason = "City project capacity reached."
		} else if playerHasProject {
			disabledReason = "You already have a project underway."
		} else if highImpactRemaining == 0 {
			disabledReason = "Daily high-impact cap reached."
		} else if p.Gold < def.CostGold {
			disabledReason = fmt.Sprintf("Need %dg.", def.CostGold)
		} else if def.CostGrain > 0 && p.Grain < def.CostGrain {
			disabledReason = fmt.Sprintf("Need %d sacks.", def.CostGrain)
		}
		projectOptions = append(projectOptions, ProjectOption{
			Type:           def.Type,
			Name:           def.Name,
			Description:    def.Description,
			CostGold:       def.CostGold,
			CostGrain:      def.CostGrain,
			DurationTicks:  def.DurationTicks,
			Disabled:       disabledReason != "",
			DisabledReason: disabledReason,
		})
	}

	var crisisView *CrisisView
	if store.ActiveCrisis != nil {
		if def, ok := crisisDefinitionByType(store.ActiveCrisis.Type); ok {
			costParts := []string{}
			if def.GoldCost > 0 {
				costParts = append(costParts, fmt.Sprintf("%dg", def.GoldCost))
			}
			if def.GrainCost > 0 {
				costParts = append(costParts, fmt.Sprintf("%d sacks", def.GrainCost))
			}
			costLabel := "Cost: none"
			if len(costParts) > 0 {
				costLabel = fmt.Sprintf("Cost: %s", strings.Join(costParts, " · "))
			}
			disabledReason := ""
			if highImpactRemaining == 0 {
				disabledReason = "Daily high-impact cap reached."
			} else if p.Gold < def.GoldCost {
				disabledReason = fmt.Sprintf("Need %dg.", def.GoldCost)
			} else if def.GrainCost > 0 && p.Grain < def.GrainCost {
				disabledReason = fmt.Sprintf("Need %d sacks.", def.GrainCost)
			}
			crisisView = &CrisisView{
				Name:                   def.Name,
				Description:            def.Description,
				Severity:               store.ActiveCrisis.Severity,
				TicksLeft:              store.ActiveCrisis.TicksLeft,
				TotalTicks:             store.ActiveCrisis.TotalTicks,
				ResponseLabel:          def.ResponseLabel,
				ResponseCost:           costLabel,
				ResponseDisabled:       disabledReason != "",
				ResponseDisabledReason: disabledReason,
			}
		}
	}

	marketBase := marketBasePrice(store.World.GrainTier)
	marketBuy := marketBuyPrice(marketBase, store.Policies.TaxRatePct, store.World.RestrictedMarketsTicks)
	marketSell := marketSellPrice(marketBase, store.Policies.TaxRatePct, store.World.RestrictedMarketsTicks)
	marketSupplySacks := store.World.GrainSupply / grainUnitPerSack
	marketMaxBuy := minInt(marketSupplySacks, p.Gold/marketBuy)
	if marketMaxBuy < 0 {
		marketMaxBuy = 0
	}
	marketMaxSell := maxInt(0, p.Grain)
	marketBuyDisabled := marketMaxBuy <= 0
	marketSellDisabled := marketMaxSell <= 0
	reliefDisabled := p.Grain < reliefSackCost
	reliefLabel := fmt.Sprintf("Fund Relief (%d sacks)", reliefSackCost)

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
			GrainStockpile:  p.Grain,
			CompletedToday:  p.CompletedContractsToday,
			CompletedTotal:  p.CompletedContracts,
			Rumors:          p.Rumors,
			PermitStatus:    permitStatus,
			AccessStatus:    accessStatus,
			WarrantStatus:   warrantStatus,
		},
		World:                   store.World,
		Situation:               store.World.Situation,
		HighImpactRemaining:     highImpactRemaining,
		HighImpactCap:           highImpactDailyCap,
		InvestigateDisabled:     investigateDisabled,
		InvestigateLabel:        investigateLabel,
		MarketBasePrice:         marketBase,
		MarketBuyPrice:          marketBuy,
		MarketSellPrice:         marketSell,
		MarketSupplySacks:       marketSupplySacks,
		MarketControlsTicks:     store.World.RestrictedMarketsTicks,
		MarketControlsActive:    store.World.RestrictedMarketsTicks > 0,
		MarketStockpile:         p.Grain,
		MarketMaxBuy:            marketMaxBuy,
		MarketMaxSell:           marketMaxSell,
		MarketBuyDisabled:       marketBuyDisabled,
		MarketSellDisabled:      marketSellDisabled,
		ReliefCost:              reliefSackCost,
		ReliefDisabled:          reliefDisabled,
		ReliefLabel:             reliefLabel,
		HasOtherPlayers:         hasOtherPlayers,
		Contracts:               contracts,
		Events:                  events,
		Players:                 players,
		Chat:                    chat,
		Messages:                messages,
		SealMessageCost:         sealedMessageCost,
		SealMessageDisabled:     sealMessageDisabled,
		SealMessageDisabledNote: sealMessageNote,
		Toast:                   toast,
		AcceptedCount:           playerAcceptedCountLocked(store, playerID),
		VisibleContractN:        len(contracts),
		TotalContractN:          totalContractN,
		Seats:                   seats,
		Policies:                store.Policies,
		Rumors:                  rumors,
		Evidence:                evidence,
		ScryReports:             scryReports,
		Intercepts:              intercepts,
		ForgeEvidenceCost:       forgeEvidenceCost,
		ForgeEvidenceDisabled:   forgeEvidenceDisabled,
		ForgeEvidenceReason:     forgeEvidenceReason,
		Loans:                   loans,
		Obligations:             obligations,
		Permits:                 permits,
		Warrants:                warrants,
		Relics:                  relics,
		RelicAppraiseCost:       relicAppraiseCost,
		Projects:                projects,
		ProjectOptions:          projectOptions,
		Crisis:                  crisisView,
		PlayerOptions:           playerOptions,
		LocationName:            locationDef.Name,
		LocationDescription:     locationDef.Description,
		LocationIconPath:        locationIconPath,
		LocationIconTint:        locationIconTint,
		Traveling:               traveling,
		TravelDestination:       travelDestination,
		TravelTicksLeft:         p.TravelTicksLeft,
		TravelTotalTicks:        p.TravelTotalTicks,
		LocationOptions:         locationOptions,
		FieldworkAvailable:      fieldworkAvailable,
		FieldworkAction:         fieldworkAction,
		FieldworkLabel:          fieldworkLabel,
		FieldworkDescription:    fieldworkDescription,
		FieldworkDisabled:       fieldworkDisabled,
		FieldworkDisabledReason: fieldworkDisabledReason,
		FieldworkSupplyCost:     fieldworkSupplyCost,
		TickStatus:              tickStatus,
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
	_ = tmpl.ExecuteTemplate(w, "diplomacy_oob", data)
	_ = tmpl.ExecuteTemplate(w, "institutions_oob", data)
	_ = tmpl.ExecuteTemplate(w, "intel_oob", data)
	_ = tmpl.ExecuteTemplate(w, "ledger_oob", data)
	_ = tmpl.ExecuteTemplate(w, "market_oob", data)
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
		_ = tmpl.ExecuteTemplate(w, "diplomacy_oob", data)
		_ = tmpl.ExecuteTemplate(w, "institutions_oob", data)
		_ = tmpl.ExecuteTemplate(w, "intel_oob", data)
		_ = tmpl.ExecuteTemplate(w, "ledger_oob", data)
		_ = tmpl.ExecuteTemplate(w, "market_oob", data)
		_ = tmpl.ExecuteTemplate(w, "toast_oob", data)
	}
}

func isAdmin(r *http.Request) bool {
	if hasValidAdminHeaderToken(r) || hasValidAdminCookieToken(r) {
		return true
	}
	if !envBool(adminLoopbackEnvName, false) {
		return false
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return host == "localhost" || (ip != nil && ip.IsLoopback())
}

func hasValidAdminHeaderToken(r *http.Request) bool {
	token := strings.TrimSpace(os.Getenv(adminTokenEnvName))
	if token == "" {
		return false
	}
	headerToken := strings.TrimSpace(r.Header.Get(adminAuthHeaderName))
	return secureStringEqual(headerToken, token)
}

func hasValidAdminCookieToken(r *http.Request) bool {
	token := strings.TrimSpace(os.Getenv(adminTokenEnvName))
	if token == "" {
		return false
	}
	c, err := r.Cookie(adminAuthCookieName)
	if err != nil || c.Value == "" {
		return false
	}
	return secureStringEqual(c.Value, signAdminCookieValue(token))
}

func signAdminCookieValue(token string) string {
	h := hmac.New(sha256.New, []byte(token))
	_, _ = io.WriteString(h, "black-granary-admin-auth-v1")
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

func secureStringEqual(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func envBool(name string, fallback bool) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(name)))
	switch v {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return fallback
	}
}

func parsePostFormLimited(w http.ResponseWriter, r *http.Request, maxBytes int64) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	if err := r.ParseForm(); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			http.Error(w, "request entity too large", http.StatusRequestEntityTooLarge)
			return false
		}
		http.Error(w, "bad request", http.StatusBadRequest)
		return false
	}
	return true
}

func issueAdminSessionCookies(w http.ResponseWriter, r *http.Request) (string, error) {
	token := strings.TrimSpace(os.Getenv(adminTokenEnvName))
	if token == "" && !envBool(adminLoopbackEnvName, false) {
		return "", fmt.Errorf("%s is required when %s is disabled", adminTokenEnvName, adminLoopbackEnvName)
	}
	secure := envBool(adminCookieSecureEnvName, true)
	if hasValidAdminHeaderToken(r) {
		http.SetCookie(w, &http.Cookie{
			Name:     adminAuthCookieName,
			Value:    signAdminCookieValue(token),
			Path:     "/admin",
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
			Secure:   secure,
		})
	}
	csrfToken, err := generateIDFromRandomBytes(32)
	if err != nil {
		return "", err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     adminCSRFCookieName,
		Value:    csrfToken,
		Path:     "/admin",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   secure,
	})
	return csrfToken, nil
}

func generateIDFromRandomBytes(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func validateAdminCSRF(r *http.Request) bool {
	c, err := r.Cookie(adminCSRFCookieName)
	if err != nil || c.Value == "" {
		return false
	}
	provided := strings.TrimSpace(r.Header.Get(adminCSRFHeaderName))
	if provided == "" {
		provided = strings.TrimSpace(r.FormValue("csrf_token"))
	}
	return secureStringEqual(provided, c.Value)
}

func buildAdminDiagnosticsLocked(store *Store, now time.Time) AdminDiagnostics {
	diag := AdminDiagnostics{
		TotalPlayers:       len(store.Players),
		OnlinePlayers:      len(onlinePlayersLocked(store, now)),
		WorldPressureLevel: "Normal",
		Alerts:             []string{},
	}
	if store.World.GrainTier == "Critical" || store.World.UnrestTier == "Unstable" {
		diag.WorldPressureLevel = "Severe"
	} else if store.World.GrainTier == "Scarce" || store.World.UnrestTier == "Uneasy" {
		diag.WorldPressureLevel = "Elevated"
	}

	repSum := 0
	heatSum := 0
	richestShareBase := 0
	for _, p := range store.Players {
		diag.TotalGold += p.Gold
		diag.TotalGrain += p.Grain
		repSum += p.Rep
		heatSum += p.Heat
		if p.TravelTicksLeft > 0 {
			diag.TravelingPlayers++
		}
		if p.Gold > diag.RichestGold {
			diag.RichestGold = p.Gold
			diag.RichestPlayer = p.Name
		}
		if diag.HottestPlayer == "" || p.Heat > diag.HottestHeat {
			diag.HottestHeat = p.Heat
			diag.HottestPlayer = p.Name
		}
		if diag.HighestRepPlayer == "" || p.Rep > diag.HighestRep {
			diag.HighestRep = p.Rep
			diag.HighestRepPlayer = p.Name
		}
		if diag.LowestRepPlayer == "" || p.Rep < diag.LowestRep {
			diag.LowestRep = p.Rep
			diag.LowestRepPlayer = p.Name
		}
		if p.Gold > 0 {
			richestShareBase += p.Gold
		}
	}

	if diag.TotalPlayers > 0 {
		diag.AvgGoldPerPlayer = diag.TotalGold / diag.TotalPlayers
		diag.AvgGrainPerPlayer = diag.TotalGrain / diag.TotalPlayers
		diag.AvgRep = repSum / diag.TotalPlayers
		diag.AvgHeat = heatSum / diag.TotalPlayers
	}
	if richestShareBase > 0 && diag.RichestGold > 0 {
		diag.RichestSharePct = (diag.RichestGold * 100) / richestShareBase
	}

	for _, c := range store.Contracts {
		switch c.Status {
		case "Issued":
			diag.ContractsIssued++
		case "Accepted":
			diag.ContractsAccepted++
		case "Fulfilled":
			diag.ContractsFulfilled++
		case "Failed":
			diag.ContractsFailed++
		}
		if (c.Status == "Issued" || c.Status == "Accepted") && c.DeadlineTicks <= 0 {
			diag.OverdueActiveContracts++
		}
		if c.Status == "Accepted" && c.OwnerPlayerID == "" {
			diag.ContractStateAnomalies++
		}
		if c.Status == "Issued" && c.OwnerPlayerID != "" {
			diag.ContractStateAnomalies++
		}
	}

	for _, loan := range store.Loans {
		if loan.Status == "Active" && loan.Remaining > 0 && loan.DueTick < store.TickCount {
			diag.OverdueActiveLoans++
		}
	}
	for _, ob := range store.Obligations {
		if ob.Status == "Open" && ob.DueTick < store.TickCount {
			diag.OverdueOpenObligations++
		}
	}

	if diag.WorldPressureLevel == "Severe" {
		diag.Alerts = append(diag.Alerts, fmt.Sprintf("World pressure is severe (grain=%s unrest=%s).", store.World.GrainTier, store.World.UnrestTier))
	}
	if diag.RichestSharePct >= 60 && diag.TotalPlayers >= 3 {
		diag.Alerts = append(diag.Alerts, fmt.Sprintf("Wealth concentration high: %s controls %d%% of positive gold.", diag.RichestPlayer, diag.RichestSharePct))
	}
	if diag.AvgHeat >= wantedHeatThreshold {
		diag.Alerts = append(diag.Alerts, fmt.Sprintf("Average heat is high (%d), enforcement pressure likely too strong.", diag.AvgHeat))
	}
	if diag.ContractsFailed >= diag.ContractsFulfilled+3 && diag.ContractsFailed >= 5 {
		diag.Alerts = append(diag.Alerts, fmt.Sprintf("Contract failure skew: %d failed vs %d fulfilled.", diag.ContractsFailed, diag.ContractsFulfilled))
	}
	if diag.OverdueActiveContracts > 0 {
		diag.Alerts = append(diag.Alerts, fmt.Sprintf("%d active contracts are overdue (deadline <= 0).", diag.OverdueActiveContracts))
	}
	if diag.ContractStateAnomalies > 0 {
		diag.Alerts = append(diag.Alerts, fmt.Sprintf("%d contract ownership inconsistencies detected.", diag.ContractStateAnomalies))
	}
	if diag.OverdueActiveLoans > 0 || diag.OverdueOpenObligations > 0 {
		diag.Alerts = append(diag.Alerts, fmt.Sprintf("Debt backlog: %d overdue loans, %d overdue obligations.", diag.OverdueActiveLoans, diag.OverdueOpenObligations))
	}
	if len(store.Warrants) > len(store.Players)/2 && len(store.Players) > 0 {
		diag.Alerts = append(diag.Alerts, fmt.Sprintf("Warrants cover %d of %d players; legal pressure may be overtuned.", len(store.Warrants), len(store.Players)))
	}
	if len(diag.Alerts) == 0 {
		diag.Alerts = append(diag.Alerts, "No immediate anomaly triggers from baseline checks.")
	}
	diag.AlertCount = len(diag.Alerts)
	return diag
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
	case heat <= 3:
		return "Noticed"
	case heat <= 7:
		return "Watched"
	case heat <= 11:
		return "Wanted"
	default:
		return "Hunted"
	}
}

func isWantedHeat(heat int) bool {
	return heat >= wantedHeatThreshold
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

func marketBasePrice(tier string) int {
	switch tier {
	case "Tight":
		return 3
	case "Scarce":
		return 5
	case "Critical":
		return 7
	default:
		return 2
	}
}

func marketBuyPrice(base, taxRatePct, controlsTicks int) int {
	price := base + int(math.Ceil(float64(base)*float64(taxRatePct)/100.0))
	if controlsTicks > 0 {
		price++
	}
	return maxInt(1, price)
}

func marketSellPrice(base, taxRatePct, controlsTicks int) int {
	price := maxInt(1, base-1)
	price -= int(math.Floor(float64(price) * float64(taxRatePct) / 100.0))
	if controlsTicks > 0 {
		price = maxInt(1, price-1)
	}
	return maxInt(1, price)
}

func applyGrainSupplyDeltaLocked(store *Store, now time.Time, delta int) {
	if delta == 0 {
		return
	}
	prevTier := store.World.GrainTier
	store.World.GrainSupply = maxInt(0, store.World.GrainSupply+delta)
	store.World.GrainTier = grainTierFromSupply(store.World.GrainSupply)
	if store.World.GrainTier != prevTier {
		addEventLocked(store, Event{Type: "Grain", Severity: 2, Text: grainTierNarrative(prevTier, store.World.GrainTier), At: now})
	}
}

func projectDefinitions() []ProjectDefinition {
	return []ProjectDefinition{
		{
			Type:          "granary_reinforcement",
			Name:          "Granary Reinforcement",
			Description:   "Expand storage and repair leakage.",
			CostGold:      10,
			CostGrain:     4,
			DurationTicks: 3,
			GrainDelta:    60,
			UnrestDelta:   -4,
		},
		{
			Type:          "civic_patrols",
			Name:          "Civic Patrols",
			Description:   "Fund watch patrols to cool hot streets.",
			CostGold:      8,
			CostGrain:     0,
			DurationTicks: 2,
			UnrestDelta:   -6,
			HeatDelta:     -2,
		},
		{
			Type:          "public_festival",
			Name:          "Public Festival",
			Description:   "Sponsor a sanctioned feast to lift morale.",
			CostGold:      6,
			CostGrain:     2,
			DurationTicks: 2,
			UnrestDelta:   -5,
			RepDelta:      2,
		},
		{
			Type:             "ward_lanterns",
			Name:             "Ward Lanterns",
			Description:      "Raise luminous wards to dampen rumors and scrying.",
			CostGold:         7,
			CostGrain:        1,
			DurationTicks:    2,
			WardNetworkTicks: 3,
		},
	}
}

func projectDefinitionByType(projectType string) (ProjectDefinition, bool) {
	for _, def := range projectDefinitions() {
		if def.Type == projectType {
			return def, true
		}
	}
	return ProjectDefinition{}, false
}

func projectEffectNote(def ProjectDefinition) string {
	parts := make([]string, 0, 4)
	if def.GrainDelta != 0 {
		parts = append(parts, fmt.Sprintf("%+d grain", def.GrainDelta))
	}
	if def.UnrestDelta != 0 {
		parts = append(parts, fmt.Sprintf("%+d unrest", def.UnrestDelta))
	}
	if def.RepDelta != 0 {
		parts = append(parts, fmt.Sprintf("%+d rep", def.RepDelta))
	}
	if def.HeatDelta != 0 {
		parts = append(parts, fmt.Sprintf("%+d heat", def.HeatDelta))
	}
	if def.WardNetworkTicks > 0 {
		parts = append(parts, fmt.Sprintf("warded veil %dt", def.WardNetworkTicks))
	}
	if len(parts) == 0 {
		return "no clear effect"
	}
	return strings.Join(parts, ", ")
}

func crisisDefinitions() []CrisisDefinition {
	return []CrisisDefinition{
		{
			Type:               "plague",
			Name:               "Grey Plague",
			Description:        "Fever grips the wards; healers plead for quarantine supplies.",
			DurationTicks:      4,
			BaseSeverity:       2,
			GoldCost:           4,
			GrainCost:          1,
			ResponseLabel:      "Fund Quarantine",
			TickUnrestDelta:    3,
			TickGrainDelta:     -4,
			ResolveRepDelta:    1,
			ResolveUnrestDelta: 3,
			FailureUnrestDelta: 6,
			FailureGrainDelta:  -12,
		},
		{
			Type:               "fire",
			Name:               "Warehouse Inferno",
			Description:        "Docks blaze; smoke chokes the market lanes.",
			DurationTicks:      3,
			BaseSeverity:       2,
			GoldCost:           3,
			GrainCost:          2,
			ResponseLabel:      "Deploy Bucket Brigade",
			TickUnrestDelta:    2,
			TickGrainDelta:     -8,
			ResolveRepDelta:    1,
			ResolveUnrestDelta: 2,
			FailureUnrestDelta: 5,
			FailureGrainDelta:  -15,
		},
		{
			Type:               "collapse",
			Name:               "Canal Collapse",
			Description:        "A canal wall fails; cargo routes grind to a halt.",
			DurationTicks:      3,
			BaseSeverity:       3,
			GoldCost:           6,
			GrainCost:          0,
			ResponseLabel:      "Hire Masons",
			TickUnrestDelta:    3,
			TickGrainDelta:     -5,
			ResolveRepDelta:    2,
			ResolveUnrestDelta: 3,
			FailureUnrestDelta: 7,
			FailureGrainDelta:  -10,
		},
	}
}

func crisisDefinitionByType(crisisType string) (CrisisDefinition, bool) {
	for _, def := range crisisDefinitions() {
		if def.Type == crisisType {
			return def, true
		}
	}
	return CrisisDefinition{}, false
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
	if c != nil {
		switch c.Type {
		case "Smuggling":
			baseGold = 35
		case "Bounty":
			if c.BountyReward > 0 {
				baseGold = c.BountyReward
			} else {
				baseGold = 28
			}
		case "Supply":
			if c.RewardGold > 0 {
				baseGold = c.RewardGold
			}
		}
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
	if c != nil && c.Type == "Bounty" {
		return 6
	}
	return 8
}

func baseContractHeatDelta(c *Contract) int {
	if c != nil && c.Type == "Smuggling" {
		return 1
	}
	if c != nil && c.Type == "Bounty" {
		return -2
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

	if c != nil && c.Type == "Bounty" {
		return DeliverOutcome{
			RewardGold: reward,
			HeatDelta:  heatDelta,
			RepDelta:   repDelta,
			Stance:     contractStanceCareful,
		}
	}
	if c != nil && c.Type == "Supply" {
		return DeliverOutcome{
			RewardGold: c.RewardGold,
			HeatDelta:  -1,
			RepDelta:   6,
			Stance:     contractStanceCareful,
		}
	}

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
