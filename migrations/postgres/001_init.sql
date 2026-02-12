CREATE TABLE IF NOT EXISTS world_state (
    id BIGINT PRIMARY KEY CHECK (id = 1),
    payload JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS policy_state (
    id BIGINT PRIMARY KEY CHECK (id = 1),
    payload JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS runtime_state (
    id BIGINT PRIMARY KEY CHECK (id = 1),
    payload JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS players (
    player_id TEXT PRIMARY KEY,
    last_seen TIMESTAMPTZ NOT NULL,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    soft_deleted_at TIMESTAMPTZ,
    hard_deleted_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS institutions (
    id TEXT PRIMARY KEY,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS seats (
    id TEXT PRIMARY KEY,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS contracts (
    contract_id TEXT PRIMARY KEY,
    status TEXT NOT NULL CHECK (status IN ('Issued', 'Accepted')),
    owner_player_id TEXT,
    issued_at_tick BIGINT NOT NULL,
    deadline_ticks BIGINT NOT NULL,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    terminal_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS permits (
    player_id TEXT PRIMARY KEY,
    expires_tick BIGINT NOT NULL,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS warrants (
    player_id TEXT PRIMARY KEY,
    expires_tick BIGINT NOT NULL,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS rumors (
    id BIGINT PRIMARY KEY,
    expires_tick BIGINT NOT NULL,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS evidence (
    id BIGINT PRIMARY KEY,
    expires_tick BIGINT NOT NULL,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS scry_reports (
    id BIGINT PRIMARY KEY,
    owner_player_id TEXT NOT NULL,
    expires_tick BIGINT NOT NULL,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS intercepts (
    id BIGINT PRIMARY KEY,
    owner_player_id TEXT NOT NULL,
    expires_tick BIGINT NOT NULL,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS loans (
    id TEXT PRIMARY KEY,
    status TEXT NOT NULL,
    due_tick BIGINT NOT NULL,
    terminal_at TIMESTAMPTZ,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS obligations (
    id TEXT PRIMARY KEY,
    status TEXT NOT NULL,
    due_tick BIGINT NOT NULL,
    terminal_at TIMESTAMPTZ,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS projects (
    id TEXT PRIMARY KEY,
    owner_player_id TEXT NOT NULL,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS active_crisis (
    id BIGINT PRIMARY KEY CHECK (id = 1),
    payload JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS relics (
    id BIGINT PRIMARY KEY,
    owner_player_id TEXT NOT NULL,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS events (
    id BIGINT PRIMARY KEY,
    at_ts TIMESTAMPTZ NOT NULL,
    day_number BIGINT NOT NULL,
    subphase TEXT NOT NULL,
    type TEXT NOT NULL,
    severity BIGINT NOT NULL,
    text TEXT NOT NULL,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS chat_messages (
    id BIGINT PRIMARY KEY,
    at_ts TIMESTAMPTZ NOT NULL,
    kind TEXT NOT NULL,
    from_player_id TEXT,
    to_player_id TEXT,
    text TEXT NOT NULL,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS diplomatic_messages (
    id BIGINT PRIMARY KEY,
    at_ts TIMESTAMPTZ NOT NULL,
    from_player_id TEXT NOT NULL,
    to_player_id TEXT NOT NULL,
    subject TEXT NOT NULL,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
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
