# Emergent World Project – Canonical Design & Development Context

This document is the **authoritative shared context** for the Emergent World project. It is intended to be reused across future AI sessions, by human collaborators, and as onboarding material for continued development.

The project explores **emergent narrative generation** through simple systemic rules, social pressure, and time-bound contracts, evolving toward a text-first, socially driven MMO-like experience.

---

## 1. Core Vision

### 1.1 What This Project Is
A **text-first, system-driven world simulator** where:
- Scarcity, unrest, and institutional responses interact
- Narrative emerges from rules, not scripts
- Players influence outcomes indirectly through contracts and choices
- The world continues to evolve even without constant player input

The long-term direction resembles a **high-fantasy / political sandbox MMO**, but the current focus is **mechanical clarity and emergence**, not content volume.

### 1.2 What This Project Is Not
- Not a scripted story engine
- Not a traditional quest-based RPG
- Not a real-time action game
- Not dependent on graphical fidelity

---

## 2. Development Phases

### Phase 1 – CLI Proof of Concept (Completed)
**Purpose:** Validate emergent behavior with no UI complexity.

- Terminal-based Go program
- Manual `advance` command to progress time
- Deterministic + bounded simulation rules
- Append-only narrative event log

Outcome:
- Confirmed that simple scarcity + unrest + contracts produce narrative arcs

---

### Phase 2 – Web Prototype (Current)
**Purpose:** Validate interaction, persistence, and cause → effect via UI.

- Go `net/http` server
- Server-side HTML rendering (`html/template`)
- htmx for dynamic partial updates
- In-memory persistence only (no database)
- Single shared world (no auth, no sessions)

Key properties:
- Clear feedback after every action
- Visible time pressure (deadlines, escalation)
- Developer-only controls allowed (e.g. Advance Time)

---

### Phase 3+ – Gameplay Evolution (Future)
Planned but **not implemented yet**:
- Removal of explicit “Advance Time” control
- Time progression via:
  - player actions
  - background ticks
  - NPC-driven events
- Multiple players influencing the same world
- Replacement of manual ticks with server-side scheduling

---

## 3. Core Simulation Model

### 3.1 Time

- Time advances in **ticks**
- Each day has two subphases:
  - Morning
  - Evening

Tick semantics:
- Morning → Evening (same day)
- Evening → Morning + DayNumber++

In Phase 2:
- Every player action triggers exactly **one tick**
- “Advance Time” is a **prototype-only debug action**

---

### 3.2 Resources – Grain

- Single abstract resource: `GrainSupply`
- Grain is consumed every tick
- Rare shocks introduce volatility

Grain tiers:
- Stable   (>200)
- Tight    (101–200)
- Scarce   (41–100)
- Critical (≤40)

Tier changes generate narrative events.

---

### 3.3 Market Pressure

- Internal price pressure derived from GrainTier
- Multipliers influence unrest only
- Numeric values are **never shown in narrative text**

Market restriction (City Authority action):
- Temporarily reduces pressure
- Has decay over time

---

### 3.4 Unrest

- Scalar value clamped 0–100
- Derived tier:
  - Calm (≤10)
  - Uneasy (≤30)
  - Unstable (≤60)
  - Rioting (>60)

Unrest increases due to:
- High market pressure
- Sustained critical scarcity
- Failed contracts

Unrest tier changes generate narrative events.

---

## 4. Factions

### 4.1 City Authority
Represents institutional power.

Triggers:
- Unrest ≥ Unstable
- GrainTier == Critical

Actions:
- Issue Emergency Contracts
- Restrict markets when rioting

Design intent:
- Stabilize at social cost
- Late or heavy-handed responses can worsen unrest

---

### 4.2 Merchant League
Represents profit-driven actors.

Triggers:
- GrainTier == Scarce or Critical

Actions:
- Issue Smuggling Contracts

Design intent:
- Exploit scarcity
- Provide alternative (often risky) solutions

---

## 5. Contracts

Contracts are the **primary player interaction surface**.

Properties:
- Type: Emergency or Smuggling
- Deadline (ticks)
- Status lifecycle:
  - Issued
  - Accepted
  - Ignored
  - Fulfilled
  - Failed

Rules:
- Contracts tick down automatically
- Can succeed or fail without player action
- Failure has strong negative systemic impact

Design role:
- Translate world pressure into player-relevant choices
- Create time pressure and moral tradeoffs

---

## 6. Player Actions (Phase 2)

Available actions:
- Advance Time (prototype-only)
- Accept Contract
- Ignore Contract
- Investigate
- Deliver

Principles:
- Every action has **visible consequences**
- Actions consume time (trigger ticks)
- Player does not directly control the world, only nudges it

---

## 7. Event System

Events are:
- Append-only
- Narrative-only (no raw numbers)
- Generated only on **meaningful changes**

Event triggers:
- Tier changes (grain or unrest)
- Faction actions
- Contract issuance
- Contract failure
- Player actions

Events are the **primary storytelling output** of the system.

---

## 8. Web Prototype Architecture (Phase 2)

### 8.1 Stack

- Go standard library (`net/http`)
- `html/template`
- htmx (CDN)
- In-memory state with `sync.Mutex`

No frameworks, no database, no client-side state.

---

### 8.2 UI Structure

Single-page dashboard with:
- World status panel
- Contracts panel
- Event log

Dynamic updates via:
- htmx `hx-post` / `hx-target`
- Partial HTML responses
- `hx-swap-oob` for event log updates

---

### 8.3 Persistence Model

- One global world state
- Shared across all users
- Resets on server restart

Intentional limitation to focus on behavior, not infrastructure.

---

## 9. Prototype-Phase Testing Goals

What must be validated before moving on:

- htmx partial updates work reliably
- In-memory state persists across requests
- Cause → effect is legible in UI
- Contracts expire and fail without player input
- Escalation emerges within ~10–15 actions
- System remains stable under repeated interaction

If these fail, numbers or rules must be tuned before adding scope.

---

## 10. Design Principles (Do Not Break)

- **Emergence over scripting**
- **Systems first, content later**
- **Scarcity creates politics**
- **Time pressure drives narrative**
- **UI exists to reveal the system, not hide it**

---

## 11. Known Temporary / Prototype-Only Elements

These are intentional scaffolding and should be removed later:
- “Advance Time” button
- Single-user assumption
- No persistence beyond memory
- Simplified probabilities

---

## 12. Next Logical Steps (Post-Prototype)

Once Phase 2 is validated:
- Remove manual time advancement
- Introduce background ticking
- Add more factions or pressures
- Replace random fulfillment with spatial/logistical constraints
- Introduce player-to-player indirect interaction via shared world

---

## 13. How to Use This Document

- Paste into a new AI session as **project context**
- Commit to repository as `PROJECT_CONTEXT.md`
- Treat as the canonical design reference
- Update deliberately; avoid silent drift

---

**This document intentionally favors clarity and continuity over brevity.**
It exists to prevent design loss, accidental simplification, or misinterpretation as the project evolves.