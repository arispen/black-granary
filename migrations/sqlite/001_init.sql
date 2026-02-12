CREATE TABLE IF NOT EXISTS world_state (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    payload TEXT NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS policy_state (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    payload TEXT NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS runtime_state (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    payload TEXT NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS players (
    player_id TEXT PRIMARY KEY,
    last_seen TIMESTAMP NOT NULL,
    payload TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    soft_deleted_at TIMESTAMP,
    hard_deleted_at TIMESTAMP
);

CREATE TABLE IF NOT EXISTS institutions (
    id TEXT PRIMARY KEY,
    payload TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS seats (
    id TEXT PRIMARY KEY,
    payload TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS contracts (
    contract_id TEXT PRIMARY KEY,
    status TEXT NOT NULL CHECK (status IN ('Issued', 'Accepted')),
    owner_player_id TEXT,
    issued_at_tick INTEGER NOT NULL,
    deadline_ticks INTEGER NOT NULL,
    payload TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    terminal_at TIMESTAMP
);

CREATE TABLE IF NOT EXISTS permits (
    player_id TEXT PRIMARY KEY,
    expires_tick INTEGER NOT NULL,
    payload TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS warrants (
    player_id TEXT PRIMARY KEY,
    expires_tick INTEGER NOT NULL,
    payload TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS rumors (
    id INTEGER PRIMARY KEY,
    expires_tick INTEGER NOT NULL,
    payload TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS evidence (
    id INTEGER PRIMARY KEY,
    expires_tick INTEGER NOT NULL,
    payload TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS scry_reports (
    id INTEGER PRIMARY KEY,
    owner_player_id TEXT NOT NULL,
    expires_tick INTEGER NOT NULL,
    payload TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS intercepts (
    id INTEGER PRIMARY KEY,
    owner_player_id TEXT NOT NULL,
    expires_tick INTEGER NOT NULL,
    payload TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS loans (
    id TEXT PRIMARY KEY,
    status TEXT NOT NULL,
    due_tick INTEGER NOT NULL,
    terminal_at TIMESTAMP,
    payload TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS obligations (
    id TEXT PRIMARY KEY,
    status TEXT NOT NULL,
    due_tick INTEGER NOT NULL,
    terminal_at TIMESTAMP,
    payload TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS projects (
    id TEXT PRIMARY KEY,
    owner_player_id TEXT NOT NULL,
    payload TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS active_crisis (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    payload TEXT NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS relics (
    id INTEGER PRIMARY KEY,
    owner_player_id TEXT NOT NULL,
    payload TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS events (
    id INTEGER PRIMARY KEY,
    at_ts TIMESTAMP NOT NULL,
    day_number INTEGER NOT NULL,
    subphase TEXT NOT NULL,
    type TEXT NOT NULL,
    severity INTEGER NOT NULL,
    text TEXT NOT NULL,
    payload TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS chat_messages (
    id INTEGER PRIMARY KEY,
    at_ts TIMESTAMP NOT NULL,
    kind TEXT NOT NULL,
    from_player_id TEXT,
    to_player_id TEXT,
    text TEXT NOT NULL,
    payload TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS diplomatic_messages (
    id INTEGER PRIMARY KEY,
    at_ts TIMESTAMP NOT NULL,
    from_player_id TEXT NOT NULL,
    to_player_id TEXT NOT NULL,
    subject TEXT NOT NULL,
    payload TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_players_last_seen ON players(last_seen);
CREATE INDEX IF NOT EXISTS idx_contracts_status_updated ON contracts(status, updated_at);
CREATE INDEX IF NOT EXISTS idx_permits_expires_tick ON permits(expires_tick);
CREATE INDEX IF NOT EXISTS idx_warrants_expires_tick ON warrants(expires_tick);
CREATE INDEX IF NOT EXISTS idx_rumors_expires_tick ON rumors(expires_tick);
CREATE INDEX IF NOT EXISTS idx_evidence_expires_tick ON evidence(expires_tick);
CREATE INDEX IF NOT EXISTS idx_scry_expires_tick ON scry_reports(expires_tick);
CREATE INDEX IF NOT EXISTS idx_intercepts_expires_tick ON intercepts(expires_tick);
CREATE INDEX IF NOT EXISTS idx_loans_status_terminal ON loans(status, terminal_at);
CREATE INDEX IF NOT EXISTS idx_obligations_status_terminal ON obligations(status, terminal_at);
CREATE INDEX IF NOT EXISTS idx_events_at ON events(at_ts);
CREATE INDEX IF NOT EXISTS idx_chat_at ON chat_messages(at_ts);
CREATE INDEX IF NOT EXISTS idx_diplomatic_at ON diplomatic_messages(at_ts);
