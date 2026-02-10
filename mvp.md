
## CODEX PROMPT — Production MVP Web MMO (Go stdlib + html/template + htmx, reset-on-restart)

### Objective

Implement a **working, fun, nice-looking (minimal elegant dark mode)** MVP web game: a small shared-world emergent simulator with contracts, event log, global chat + whispers, gold + reputation, and visible consequences. It must support **5–10 concurrent players** in one process, no database, reset-on-restart.

**Tech constraints**

* Go standard library only for server/routing: `net/http`, `http.ServeMux`
* Server-side rendering: `html/template`
* htmx via CDN only (no bundlers)
* No DB, no Redis, no external deps
* In-memory state protected by mutex
* Works on `go run .` and is playable at `http://localhost:8080`

**Core design constraints**

* Single shared world
* Time does **NOT** advance per player action (avoid acceleration with multiple players)
* World ticks on a **fixed schedule while players are online**
* Additionally, the **first player login of each UTC day triggers 1 tick** (bootstrap), but no multi-day catch-up
* Players are anonymous Guests with random fantasy names persisted via cookie
* Players can lose gold and reputation
* Engagement: “Today’s Situation” summary + reputation titles + public consequences in event log

---

# 1) Project Layout (exact)

```
main.go
templates/
  base.html
  index.html
  fragments/
    dashboard.html
    events.html
    chat.html
    players.html
    toast.html
```

---

# 2) Routes (exact)

### Full page

* `GET /`
  Full page HTML (base+index). Ensures player identity cookie exists; updates LastSeen; performs **daily tick** if first login UTC day (exact rule in section 7).

### Fragments (htmx partials)

* `GET /frag/dashboard` → returns dashboard fragment (for swapping #dashboard) + OOB updates for events/players/toast
* `GET /frag/events` → returns events fragment (for polling)
* `GET /frag/chat` → returns chat fragment (for polling, filtered to the current player for whispers)
* `GET /frag/players` → returns players fragment (for polling)
* `POST /action` → apply player action (contract accept/ignore/abandon/deliver/investigate); returns dashboard + OOB updates
* `POST /chat` → send global or whisper; returns chat + OOB updates (and optionally toast)

### Admin/debug (must exist)

* `GET /admin` → raw state snapshot + controls (local-only or token-protected)
* `POST /admin/tick` → force one world tick (admin only)
* `POST /admin/reset` → reset world (admin only)

**Admin protection**

* Simplest: allow only if `r.RemoteAddr` is localhost OR query param `?token=DEV` with constant `AdminToken = "DEV"`. Keep minimal.

---

# 3) UI + htmx Contract (exact DOM IDs)

Full page must include stable containers:

* `<div id="dashboard">...</div>` (status + contracts + action controls)
* `<div id="event-log">...</div>`
* `<div id="chat">...</div>`
* `<div id="players">...</div>`
* `<div id="toast">...</div>` (small feedback area)

htmx script in `<head>`:

```html
<script src="https://unpkg.com/htmx.org@1.9.12"></script>
```

### Main interaction pattern

All mutations use htmx POST and update dashboard:

* `hx-post="/action" hx-target="#dashboard" hx-swap="innerHTML"`

### Multi-region updates

Responses from `/action` must include:

* dashboard HTML (for `#dashboard` swap)
* OOB fragments:

  * `<div id="event-log" hx-swap-oob="innerHTML">...</div>`
  * `<div id="players" hx-swap-oob="innerHTML">...</div>`
  * `<div id="toast" hx-swap-oob="innerHTML">...</div>`
    Optionally include chat OOB if action affects chat (usually not needed).

Responses from `/chat` must include:

* `<div id="chat">...</div>` content swap (either normal or OOB)
* plus OOB events/players/toast if relevant

### Polling (simple “MMO feel”)

On the page, add:

* events poll: every 5s → `hx-get="/frag/events" hx-target="#event-log" hx-swap="innerHTML"`
* chat poll: every 2s → `hx-get="/frag/chat" hx-target="#chat" hx-swap="innerHTML"`
* players poll: every 5s → `hx-get="/frag/players" hx-target="#players" hx-swap="innerHTML"`
  Keep responses small (bounded buffers).

---

# 4) In-memory State & Concurrency (must be correct)

Use a single global store:

```go
type Store struct {
  mu sync.Mutex

  World WorldState

  Players map[string]*Player        // by playerID
  Contracts map[string]*Contract     // by contractID

  Events []Event                    // ring buffer max 300
  Chat []ChatMessage                // ring buffer max 200

  NextEventID int64
  NextContractID int64
  NextChatID int64

  LastDailyTickDate string          // "YYYY-MM-DD" UTC
  LastTickAt time.Time              // UTC
  TickEvery time.Duration           // e.g. 60s

  // anti-spam / cooldown
  LastChatAt map[string]time.Time   // by playerID
  LastActionAt map[string]time.Time // by playerID
}
```

**Locking rule:** every handler locks `mu` for its whole duration (simple + safe). Tick goroutine also locks `mu`. No data races.

**Online definition:** player is online if `now.Sub(LastSeen) <= 60s`.

---

# 5) Player Identity (Guest, cookie)

Cookie name: `pid` (HttpOnly, SameSite=Lax).

On any request:

1. If cookie missing → create player:

   * generate random ID (secure random bytes, base64url)
   * generate fantasy name from two string slices (first+last) and suffix `" (Guest)"`
   * initial Gold=20, Rep=0
2. If cookie exists but player not in map (server restarted) → recreate player with new random name (keep same pid cookie value ok)
3. Update `LastSeen = time.Now().UTC()`

**Name collision:** if generated name already used, append `#XYZ` (3 chars).

**Reputation title badge** (soft ranking):

* Rep ≥ 50: Renowned
* 20..49: Trusted
* -19..19: Unknown
* -49..-20: Shady
* ≤ -50: Notorious

Show badge in players list + chat + contracts “Taken by”.

---

# 6) World Simulation (Emergent Core)

### WorldState fields

```go
type WorldState struct {
  DayNumber int
  Subphase string // "Morning"|"Evening"

  GrainSupply int
  GrainTier string // "Stable"|"Tight"|"Scarce"|"Critical"

  UnrestValue int
  UnrestTier string // "Calm"|"Uneasy"|"Unstable"|"Rioting"

  RestrictedMarketsTicks int

  CriticalTickStreak int
  CriticalStreakPenaltyApplied bool

  // engagement / flavor
  Situation string // derived each tick
}
```

Initial:

* DayNumber=1, Subphase="Morning"
* GrainSupply=300
* UnrestValue=5
* RestrictedMarketsTicks=0

### Tick model (IMPORTANT: avoid time acceleration)

* World ticks automatically every `TickEvery = 60s` **ONLY if at least 1 player online**
* Also do the daily tick on first login UTC day (section 7)
* Player actions do NOT advance time.

### Tick semantics

Each tick toggles subphase:

* Morning → Evening (same day)
* Evening → Morning and DayNumber++

### Grain update per tick

* Consume: `GrainSupply -= (18 + randInt(0..8))`, clamp min 0
* Shocks:

  * 10% spoilage: -25 grain (clamp 0)
  * 8% caravan: +20 grain if mid-tick GrainTier != Stable
* Tier thresholds:

  * Stable >200
  * Tight 101..200
  * Scarce 41..100
  * Critical 0..40

### Market pressure (internal)

Base multiplier:

* Stable 1.0, Tight 1.5, Scarce 2.0, Critical 3.0
  If `RestrictedMarketsTicks > 0`, EffectiveMultiplier = max(1.0, base-0.5)
  Never show multipliers numerically.

### Unrest update per tick

* If EffectiveMultiplier ≥ 2.0 → +5 unrest
* Contract failures this tick → +15 each
* Contract fulfillments this tick → -10 each
* Critical streak: if final GrainTier == Critical:

  * `CriticalTickStreak++`, else reset streak + penalty flag
  * When streak reaches 4 and penalty not applied: +10 unrest once, set flag
    Clamp UnrestValue 0..100
    Unrest tiers:
* Calm ≤10, Uneasy ≤30, Unstable ≤60, Rioting >60

### Factions (auto actions per tick)

City Authority triggers if (UnrestTier >= Unstable) OR (GrainTier == Critical)

* If no active Emergency contract exists → issue one with DeadlineTicks=4
* If UnrestTier == Rioting and RestrictedMarketsTicks == 0 → set RestrictedMarketsTicks=2

Merchant League triggers if GrainTier in {Scarce, Critical}

* If no active Smuggling contract exists → issue one with DeadlineTicks=3

**Active contract definition for “no active exists”:**
Contract status in {Issued, Accepted} counts as active. Ignored does NOT count as active.

### Contracts resolution per tick (global)

Contract:

```go
type Contract struct {
  ID string
  Type string // "Emergency"|"Smuggling"
  DeadlineTicks int
  Status string // "Issued"|"Accepted"|"Fulfilled"|"Failed"|"Ignored"
  OwnerPlayerID string // if Accepted
  OwnerName string // cached display name for UI
  IssuedAtTick int64 // optional
}
```

Each tick for each contract with Status in {Issued, Accepted, Ignored}:

* If Status == Accepted: fulfillment chance = base + 15pp (cap 95)
* Base fulfill chance by GrainTier:

  * Stable 70, Tight 55, Scarce 40, Critical 25
* If fulfilled by tick:

  * Status=Fulfilled
  * Apply grain reward immediately: Emergency +60, Smuggling +30
  * (Optionally: pay owner if Accepted; see economy below)
* If not fulfilled:

  * DeadlineTicks--
  * If DeadlineTicks <= 0 → Status=Failed, generate failure event; if Accepted, owner loses rep (below)

### Grief prevention / caps (MVP guardrails)

* A player may have at most **1 Accepted contract** at a time.
* Accept is exclusive and atomic under mutex; losers see toast “Taken by X.”
* If a player accepts and then goes inactive (LastSeen > 120s), auto-abandon at next tick:

  * contract Status becomes Issued again (or Failed if Deadline <=1; choose: re-issue if still time)
  * event: “A contractor vanishes; the job returns to the board.”
* “Ignore” sets status Ignored but does NOT prevent issuance of new faction contracts (ignored doesn’t count as active).
* Deliver attempts are limited: **at most 1 deliver attempt per player per 10 seconds** (cooldown).

### Events: narrative-only, no numbers in Text

```go
type Event struct {
  ID int64
  DayNumber int
  Subphase string
  Type string
  Severity int
  Text string
  At time.Time
}
```

Event log is ring buffer max 300; keep chronological order; display newest last (or first) consistently.

Event triggers:

* Grain tier change
* Unrest tier change
* Faction issues contract
* City restricts markets
* Contract failed
* Daily tick message
* Player actions (accept/ignore/abandon/deliver/investigate)
* Optional flavor: low-probability “atmosphere” line when no events occurred

Narrative mappings (no numeric values):
Grain tier change:

* Tight: “Grain supply tightens.”
* Scarce: “Grain stores thin across the city.”
* Critical: “Grain stores fall below emergency reserves.”
* Upward recovery: “Fresh grain reaches the markets, easing shortages.”

Unrest tier change:

* Uneasy: “Whispers of worry spread through the streets.”
* Unstable: “Tension rises as crowds gather and tempers flare.”
* Rioting: “The city erupts into open unrest.”
* Downward: “The streets quiet as tensions ease.”

Faction actions:

* Emergency issued: “[City Authority] requisitions emergency shipments.”
* Markets restricted: “[City Authority] imposes strict market controls.”
* Smuggling issued: “[Merchant League] issues smuggling orders.”

Contract failed:

* “A contract has failed, raising tension in the city.”

Daily tick:

* “A new day dawns with fresh uncertainty.”

Player action templates:

* Accept: “[<Player>] commits to a dangerous contract.”
* Ignore: “[<Player>] turns away as pressure mounts.”
* Abandon: “[<Player>] abandons a claim as the city watches.”
* Investigate: “[<Player>] investigates rumors along the supply routes.”
* Deliver success: “[<Player>] pushes a delivery through tense streets.”
* Deliver fail: “[<Player>] attempts a delivery, but it collapses at the last moment.”

---

# 7) Daily Tick Rule (first login of UTC day)

On `GET /`:

* todayUTC := now.UTC().Format("2006-01-02")
* if Store.LastDailyTickDate != todayUTC:

  * run exactly 1 world tick immediately
  * append daily tick event (above)
  * set Store.LastDailyTickDate = todayUTC
    No multi-day catch-up.

---

# 8) Economy (Gold & Reputation)

Player:

```go
type Player struct {
  ID string
  Name string
  Gold int
  Rep int // clamp [-100..100]
  LastSeen time.Time
}
```

Gold rules:

* Gold never negative (clamp at 0)
* Gold sinks exist to prevent inflation:

  * Deliver attempt costs 2 gold (always, even if fails)
  * Optional “bribe” action not in MVP (skip)

Reputation rules:

* Rep clamps to [-100..100]
* Rep affects contract eligibility:

  * Rep < -50: can only see Emergency contracts (no Smuggling)
  * Rep > 30: sees “special” higher payout variants (optional)
* Rep affects payouts slightly:

  * payoutMultiplier = 1.0 + (Rep/200.0) (so at +100 => +0.5x), clamp between 0.75 and 1.5

Contract payouts (on fulfillment):

* If contract was Accepted by a player:

  * Emergency: +25 gold, +8 rep
  * Smuggling: +35 gold, +3 rep
    Apply multiplier to gold only (not rep).
* If contract fulfills without an owner (auto-fulfill), no one gets gold/rep.

Failure consequences:

* If an Accepted contract fails by deadline:

  * Owner Rep -= 10 (and world unrest increase already happens)
* If deliver attempt fails:

  * Owner Rep -= 5
* Investigate:

  * UnrestValue -= 5 (clamp), Rep += 1
    Investigate anti-spam:
* Only effective once per player per 3 ticks (track per player “lastInvestigateTick”; otherwise it just posts a flavor event).

---

# 9) Player Actions (POST /action)

Form fields:

* `action` string
* optional `contract_id`

Actions:

* `accept`: exclusive accept if Issued and player has <1 accepted contract; else toast
* `ignore`: only if Issued and not owned; set Ignored
* `abandon`: only if Accepted and owned by player; set Issued, clear owner
* `deliver`: only if Accepted and owned by player; apply gold cost; attempt fulfillment:

  * Chance by GrainTier:

    * Stable 80, Tight 65, Scarce 50, Critical 35
  * If success: mark Fulfilled, apply grain reward + payouts, add event
  * If fail: add event + rep penalty
* `investigate`: apply once-per-3-ticks effectiveness; otherwise flavor-only

**Action rate limiting:**

* enforce 1 action per player per 2 seconds (server side). If too fast, toast “Slow down.”

**Important:** actions do NOT trigger world tick. Tick is scheduler-driven.

---

# 10) Chat (POST /chat, polling fragments)

Chat message:

```go
type ChatMessage struct {
  ID int64
  FromPlayerID string
  FromName string
  ToPlayerID string // empty => global
  ToName string
  Text string
  At time.Time
  Kind string // "global"|"whisper"|"system"
}
```

Rules:

* Rate limit: 1 msg per player per 2 seconds
* Command:

  * If text starts with `/w ` then parse `/w <Name> <message>`
  * Resolve Name case-insensitive exact match to a player’s display name WITHOUT “(Guest)” suffix OR with it; implement robust matching:

    * normalize by lowercasing and trimming spaces
    * allow whisper target input without “(Guest)”
  * If not found: return a system message visible only to sender (store it as whisper to self)
* Global chat visible to all
* Whispers visible only to sender and recipient
* Escape all chat text in templates (html/template does)

---

# 11) “Fun / engagement” UI elements (must implement)

### Today’s Situation (dashboard top)

Derive a short summary string each tick from GrainTier + UnrestTier:
Examples:

* Stable/Calm: “The city breathes—uneasy peace holds.”
* Scarce/Uneasy: “Shortages spread quiet panic through the markets.”
* Critical/Unstable: “Hunger sharpens into anger; deals turn desperate.”
* Critical/Rioting: “The streets burn with desperation and blame.”

Display prominently.

### Urgency styling

Contracts show deadline ticks and urgency class:

* deadline <=1: “urgent”
* <=2: “warning”
* else normal
  Use CSS to highlight.

### Public consequences

Event log includes player action events (accept/deliver/fail/abandon) so others see outcomes.

---

# 12) Templates & Styling (dark, minimal elegant)

Implement a clean dark UI with simple CSS in base.html:

* background near #0b0f14
* panels/cards with slightly lighter background
* subtle borders, rounded corners, consistent spacing
* readable font stack (system UI)
* buttons with hover states, disabled states
* badges for titles and contract states
  No external CSS frameworks.

Layout suggestion:

* Two-column grid on desktop:

  * Left (60%): Dashboard (situation, world status, contracts)
  * Right (40%): Events, Chat, Players stacked
* On narrow screens: stack vertically

Templates:

* `base.html`: shell + CSS + htmx + layout containers + polling hooks
* `index.html`: initial render includes inner HTML for dashboard/events/chat/players
* fragments:

  * `dashboard.html`: returns **inner content** intended for `#dashboard`
  * `events.html`: returns `<div id="event-log" ...>` (either normal or OOB)
  * `chat.html`: returns `<div id="chat" ...>`
  * `players.html`: returns `<div id="players" ...>`
  * `toast.html`: returns `<div id="toast" ...>` short feedback line

**Important:** For OOB responses, wrap updated nodes exactly with matching IDs.

---

# 13) Rendering Rules

* Parse templates at startup.
* Create helper functions:

  * `renderPage(w, tmpl, data)`
  * `renderFragmentsActionResponse(w, data)` that executes dashboard fragment, then events/players/toast with OOB attributes.
* Always set `Content-Type: text/html; charset=utf-8`

---

# 14) Tick Scheduler (must be correct)

Start a goroutine in main():

* Ticker every 1s checks if:

  * `onlineCount > 0` and `time.Since(LastTickAt) >= TickEvery`
* When due, lock store, run one tick, update LastTickAt, unlock.
* Ensure ticks cannot overlap (mutex ensures).
* When no players online, do not tick.

---

# 15) Admin Page (debug + tuning)

`GET /admin` shows:

* world raw state (numbers ok)
* active contracts list (raw)
* online players
* buttons to force tick/reset
* note “RESET ON RESTART (test realm)”

Admin actions:

* `/admin/tick` runs one tick
* `/admin/reset` resets store to initial state (new empty maps; preserve AdminToken)

---

# 16) Acceptance Checklist (must pass)

* Launch server, open `/`: name assigned & persisted via cookie.
* Dark UI looks clean and readable.
* Polling updates events/chat/players without reload.
* Tick occurs every 60s when players online; does not accelerate with multiple players.
* Daily tick triggers once per UTC day on first login.
* Contracts issue under pressure; are exclusive on accept; max 1 accepted per player.
* Deadlines count down on ticks; failures raise unrest and generate events.
* Players earn/lose gold & rep; gold not negative; rep clamps -100..100.
* Chat global works; whisper works and is private.
* Rate limits prevent spam.
* Admin works.

---

# 17) Implement Now

Implement `main.go` and all template files exactly as described. Add clear comments explaining:

* htmx swapping strategy
* OOB updates
* tick scheduler and daily tick
* concurrency/locking choices
* why actions don’t advance world time

Do not add any dependencies besides htmx CDN.

