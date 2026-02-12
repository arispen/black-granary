# Release Notes

## 0.22.0
- Added SQL-backed persistence with dialect switching via `DB_DIALECT=sqlite|postgres`.
- Added parallel dialect migrations under `migrations/sqlite` and `migrations/postgres` with aligned logical schema.
- Added daily retention cleanup job for events/chat/diplomatic messages plus loan/obligation/player lifecycle retention.
- Kept gameplay/handler logic dialect-agnostic while isolating driver and migration differences in the DB adapter.
- Persisted only active contracts (`Issued`, `Accepted`), with narrative outcomes retained in events.

## 0.21.0
- Added colorful, CSS-tinted game icons across core panels (dashboard, events, chat, diplomacy, players, institutions, intel, ledger, market).
- Introduced dynamic location and contract-type icons in the dashboard for faster visual scanning.
- Added static `/assets/` serving so icon packs in the repository are delivered directly by the Go server.

## 0.20.0
- Added Commander of the Watch seat with warrant issuance and time-bound warrant tracking.
- Warranted targets now spawn boosted bounty rewards and show warrant status in the UI.
- Contracts panel now calls out travel lockouts, and Institutions lists active warrants.

## 0.19.0
- Added sealed missives to diplomacy with a gold cost and seal indicators.
- Courier intercepts now penalize sealed messages and redact sealed intel.
- Threaten exposure no longer consumes high-impact budget without evidence.

## 0.18.0
- Bribes now grant temporary corrupt access that bypasses permits and embargoes.
- Contract cards surface embargo/permit requirements plus bribed access notes.
- Dashboard standing shows access status and disables actions while traveling.

## 0.17.0
- Added High Curate inquest action to purge forged evidence and dampen rumor waves on a target.
- Institutions/Offices now surface the Conduct Inquest control with high-impact budgeting.
- Inquests boost the target's standing while logging doctrine events.

## 0.16.0
- Added forged evidence intel action with costed, short-lived dossiers and failure penalties.
- Intel panel now labels evidence origin (forged vs investigated) and shows expiry ticks.
- Forge evidence controls now communicate when high-impact or travel lockouts apply.

## 0.15.0
- Added courier intercept intel action that captures short-lived missives.
- Intel panel now lists intercepted missives with expiry timers.
- Market buy/sell inputs now disable when trading is blocked or traveling.

## 0.14.0
- Added Ward Lanterns public works project that activates a temporary ward network.
- Ward network dampens rumor spread and reduces scrying success chances while active.
- Intel, Institutions, and Ledger panels now surface travel lockouts and disable actions during travel.

## 0.13.0
- Added scrying intel action that captures short-lived dossiers on other players.
- Intel panel now lists scrying reports with location, travel, and resource snapshots.
- Publishing evidence no longer consumes high-impact budget when no evidence exists.

## 0.12.0
- Added relic recovery from the Haunted Ruins with Temple appraisals and one-time invocations.
- Relic dashboard card now shows appraisal status, effects, and action controls.
- Traveling now pauses market actions and surfaces clearer travel lockout messaging.

## 0.11.0
- Added Harbor Master permits for emergency contracts, with tick-based expiry.
- Institutions panel now lists active permits and allows issuing permits to other players.
- Contract cards now surface permit requirements and disable acceptance when blocked.

## 0.10.0
- Added travel between districts with per-player travel timers and arrival events.
- New fieldwork actions in the frontier and ruins grant rewards or risk heat/rep.
- Dashboard now shows travel status and fieldwork availability, with improved institutions/intel hints.

## 0.9.0
- Added citywide crisis events (plague, fire, collapse) that pressure unrest and grain supplies.
- Crisis Watch shows active emergencies and lets players fund response actions.
- Permit/embargo toggles now display clear action labels, and impact counters are clarified.

## 0.8.0
- Added a Diplomacy panel for private missives with courier notifications.
- New diplomacy inbox keeps messages visible only to sender and recipient.
- Institutions panel now labels permit/embargo toggles consistently.

## 0.7.0
- Added public works projects with time-bound effects on grain, unrest, reputation, and heat.
- Institutions panel now lists active projects and funding options.
- Intel actions now respect investigate cooldowns and high-impact caps in the UI.

## 0.6.0
- Added obligation costs with settle and forgive actions in the ledger.
- Overdue obligations now apply heat/rep penalties and raise unrest.
- Ledger UI shows obligation cost, status, and action states.

## 0.5.0
- Added player-posted supply contracts with escrowed rewards and sack requirements.
- Supply contracts now show issuer, reward, and requirements, with issuer withdrawal available.
- Supply deliveries consume stockpiled grain, ease unrest, and reward contractors.

## 0.4.0
- Added law pressure with Wanted heat tiers and bounty contracts for high-heat players.
- Bounty contracts require evidence delivery and penalize the target when resolved.
- Contract UI now shows bounty targets/requirements and hides stance selection where irrelevant.
- Players list shows heat levels, and market inputs handle zero-quantity states cleanly.

## 0.3.0
- Added a Market panel with buy/sell grain prices tied to scarcity, taxes, and controls.
- Introduced player stockpiles (grain sacks) and relief donations that ease unrest.
- Made intel/loan actions hide when no other players are available.
- Added market polling/OOB updates and tests for market actions.

## 0.2.2
- Improved action cooldown UX and high-impact cap messaging.
