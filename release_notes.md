# Release Notes

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
