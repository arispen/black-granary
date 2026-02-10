### **Text-Based High-Fantasy Social Sandbox MMO (Design Summary)**

This project is a concept for a **browser-based, mostly text-driven MMORPG** that blends simulation-heavy sandbox gameplay with player-driven politics, economy, and emergent storytelling. The goal is to create an MMO where the **primary source of long-term engagement is social interaction and systemic consequences**, rather than traditional PvE grinding.

The game is best described as:

**A “Game of Thrones MMO” powered by EVE-like economy/logistics, Dwarf Fortress-style simulation, Space Station 13 social chaos, and Morrowind-style lore and factions.**

It emphasizes **player agency** (in the BG3 sense) not through cinematic authored branching dialogue, but through scalable systemic mechanics.

---

# **Core Design Pillars**

## **1\. Emergent Simulation (DF / NetHack Influence)**

The world is governed by simulation systems that continuously produce outcomes:

* resource production and shortages  
* disease outbreaks  
* infrastructure failures  
* environmental hazards  
* artifact curses and magical anomalies

The simulation is designed so that **unexpected interactions generate stories**, similar to Dwarf Fortress and NetHack. Artifacts and magic are not just stat boosts; they alter rules and create cascading consequences.

**Design intent:** the world should produce drama even without players.

---

## **2\. Social Roleplay Chaos (Space Station 13 Influence)**

Players operate inside institutions and communities rather than isolated hero narratives. The game supports:

* professions and civic roles  
* betrayal and sabotage  
* departments/institutions with internal conflict  
* cooperation under pressure

SS13’s “job-based society” is translated into a persistent world context: instead of “station roles,” players occupy roles in fantasy institutions (churches, guilds, noble houses, trade leagues).

**Design intent:** the player experience should feel like living inside a society, not just adventuring.

---

## **3\. Player-Driven Economy, Logistics, and Power (EVE Influence)**

Economic systems are central. Wealth and influence come from:

* production  
* trade routes  
* supply chain control  
* speculation  
* monopolies  
* taxation and permits

The economy is not decorative—it is a strategic weapon. War and politics are fueled by material scarcity and logistics.

**Design intent:** power comes from coordination, information, and infrastructure control.

---

## **4\. High-Fantasy Lore and Immersion (Morrowind Influence)**

The setting is high fantasy but *strange* and politically layered, not generic medieval fantasy. It includes:

* competing ideologies  
* factions with beliefs, not just “teams”  
* mystery-driven exploration  
* artifacts with metaphysical meaning

Lore matters because it becomes mechanically useful: knowledge, rituals, and history are strategic resources.

**Design intent:** exploration and information should matter as much as combat.

---

## **5\. Player Agency as Systemic Choice (BG3 Influence Reinterpreted)**

The project borrows BG3’s sense of agency, but implements it through mechanics rather than authored narrative.

Agency is defined as:

* many available “verbs” (ways to solve problems)  
* meaningful consequences  
* persistent world reactions  
* branching outcomes even on failure

Instead of scripted quests, the game generates opportunities through contracts, crises, and faction conflict.

**Design intent:** players should solve the same problem via diplomacy, crime, bribery, force, trade, or manipulation.

---

# **Key Added Systems (Identified as Necessary)**

Three major missing ingredients were added to make the concept scalable and sustainable:

## **A) Contracts \+ Reputation Framework**

Contracts replace traditional questing and grinding. They are the main gameplay loop and are issued by:

* NPC factions (always active)  
* players (once population grows)

Reputation is not cosmetic; it gates:

* access to markets and restricted goods  
* faction protection or hostility  
* legal status  
* docking/entry rights  
* ability to hold office or gain titles

**Outcome:** a structured way to create content and consequences.

---

## **B) Information Warfare as First-Class Gameplay**

The game makes information a strategic resource. Mechanics include:

* forgery and false documents  
* intercepted communications  
* espionage and surveillance  
* rumor propagation with credibility/provenance  
* blackmail material and evidence systems  
* counterintelligence: audits, investigations, forensics

Fantasy equivalents include:

* illusion-based impersonation  
* divine truth rituals (imperfect/corruptible)  
* necromantic interrogation  
* prophecy markets

**Outcome:** political drama becomes gameplay, not just Discord arguments.

---

## **C) Time as a Resource**

Time is explicitly modeled. Key elements:

* travel delays and risk  
* seasons and cycles  
* deadlines for contracts and events  
* long-term projects (construction, rituals, research)  
* shifting opportunity windows

Time pressure creates opportunity cost and planning gameplay.

**Outcome:** logistics, preparation, and timing become core strategic decisions.

---

# **World Scaling Strategy (10 MAU vs 1000 MAU)**

The game is designed to function at very low population while still scaling to high population.

## **Low Population (≈10 MAU)**

The world must still feel alive and dramatic even with few players. Therefore:

* NPC factions drive politics and economic activity  
* the world simulation generates crises automatically  
* players act as “agents” within NPC-driven institutions  
* meaningful content exists without requiring player wars

**Key requirement:** NPC factions must act independently and generate opportunities.

---

## **High Population (≈1000 MAU)**

As population increases:

* players gradually replace NPC leadership  
* player organizations become the dominant political force  
* NPCs become background population and labor

The game becomes a fully player-driven political sandbox.

---

## **Mechanism: “Seats of Power”**

Each faction has a set of limited authority roles (“seats”) such as:

* Archmage of the College  
* Bishop of the Cathedral  
* Master of Coin / Trade Commissioner  
* Harbor Master / Gatekeeper of Teleport Circle  
* Commander of the Watch

At low population, NPCs occupy these seats. At higher population, players can acquire them via:

* elections  
* coups  
* bribery  
* conquest  
* trials/legal proceedings

This creates a smooth transition from NPC-driven to player-driven society.

---

# **High-Fantasy Implementation (Game of Thrones MMO Effect)**

The concept naturally resembles a “Game of Thrones MMO,” but differs by making power depend on simulation systems.

Players fight over:

* leyline access (mana infrastructure)  
* rare materials (mithril, dragonbone, holy silver)  
* relic legitimacy (crowning authority)  
* trade routes and ports  
* resurrection monopolies (church power)  
* scrying networks (intel)  
* magical ward networks (city defense)

Wars are won through supply chain disruption, assassination, propaganda, and control of infrastructure—not only battles.

---

# **Update Model (Chapters / Seasons / Expansions)**

Updates are considered essential for MMO longevity. However, the goal is to avoid treadmill “new gear tiers” progression.

The recommended model:

* expansions introduce **sideways power** (new institutions, regions, resources with tradeoffs)  
* seasons introduce instability (new crises, shifting trade routes, new political threats)  
* live events reshape the world (cataclysms, plagues, portal openings)

Updates should primarily serve to:

* introduce new scarcity  
* destabilize old power structures  
* generate new faction conflicts  
* reshape logistics and markets

---

# **Format Twist: Text-Based Browser MMO**

The project is explicitly designed as a mostly text-based browser game. This increases feasibility because:

* complex systemic agency is easier than cinematic content  
* social gameplay and politics do not require graphics  
* asynchronous play becomes natural (action queues, delayed travel)  
* NPC institutional behavior feels more acceptable in text

The game UI resembles a strategic dashboard / interactive fiction hybrid.

Key UI pages envisioned:

* Home / world event feed  
* Location page  
* Contracts board  
* Market page  
* Character sheet  
* Messages / diplomacy

Combat exists but is intentionally rare and high consequence.

---

# **MVP Design Outline (Minimal Viable Product)**

The MVP is defined as the smallest playable version that produces emergent social stories.

## **MVP Systems (Must-Have)**

1. **World map \+ time-based travel**  
2. **NPC factions \+ reputation \+ access rules**  
3. **Contract board as primary content loop**  
4. **Market system with local pricing and scarcity**  
5. **Conflict engine generating crises automatically**  
6. **Messaging/diplomacy system**

## **MVP World Size**

Start dense and small:

* 1 capital city (multiple districts)  
* 1 port town  
* 1 frontier village  
* 1 mine / resource site  
* 1 haunted ruin (PvE/exploration)  
* 1 wilderness trade route

## **MVP Gameplay Actions**

Core verbs:

* travel  
* buy/sell  
* deliver goods  
* explore ruins  
* investigate incidents  
* fight encounters  
* steal (high risk)  
* bribe  
* basic work roles for factions

## **MVP Law System**

Simple but powerful:

* zones have “law level”  
* crimes create “wanted” status  
* wanted affects markets, guards, travel safety  
* bounties generate contracts

## **MVP Progression**

Horizontal progression:

* skills unlock new actions (forgery, healing, scrying, smuggling)  
* reputation unlocks privileges  
* wealth unlocks influence  
  Avoid vertical “level treadmill.”

---

# **Design Philosophy: PvE as Fuel, Not the Game**

A core conclusion was that MMOs are strongest when:

* PvE provides scarcity and danger  
* players decide who benefits from it  
* the main gameplay is politics, economy, and social consequence

Grinding is minimized and should never be mandatory treadmill content. PvE should exist primarily to generate:

* resources  
* hazards  
* rare artifacts  
* contracts  
* instability

---

# **Key Risks and Mitigations**

## **Risk: Social-only becomes clique-driven and hostile**

Mitigation:

* contract system provides structure  
* NPC factions ensure drama at low pop  
* law/reputation systems create accountability

## **Risk: Game becomes spreadsheet optimizer**

Mitigation:

* information uncertainty and rumor mechanics  
* procedural crises that disrupt equilibrium  
* partial success outcomes instead of binary fail states

## **Risk: Zerg guild dominance at scale**

Mitigation:

* geography and logistics create fragmentation  
* small-team sabotage and espionage objectives remain impactful  
* holding territory requires upkeep and institutional management

---

# **Intended Outcome**

The MVP should naturally generate stories like:

* “We cornered the grain market during famine.”  
* “The church branded me heretic, I fled to smugglers.”  
* “We bribed the harbor master to block rival shipments.”  
* “Someone forged a royal decree and started a civil war.”  
* “A plague hit the capital and healers became kingmakers.”

If these stories emerge from systems rather than scripted content, the project is considered successful.

---

# **One-Line Pitch (Canonical)**

**A text-based fantasy MMO where the world runs on simulation, scarcity, and secrets—players build power through trade, law, religion, and betrayal, and the story is whatever society becomes.**

