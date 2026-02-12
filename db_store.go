package main

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

//go:embed migrations/sqlite/*.sql migrations/postgres/*.sql
var migrationFS embed.FS

type DBDialect string

const (
	dialectSQLite   DBDialect = "sqlite"
	dialectPostgres DBDialect = "postgres"
)

type SQLRepository struct {
	dialect DBDialect
	db      *sql.DB
}

type runtimeState struct {
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
	TickEveryNanos    int64
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
	LastCleanupDate   string
}

func newConfiguredStore() (*Store, error) {
	store := newStore()
	repo, err := openRepositoryFromEnv()
	if err != nil {
		return nil, err
	}
	if repo == nil {
		return store, nil
	}
	store.repo = repo
	if err := repo.LoadInto(context.Background(), store); err != nil {
		return nil, err
	}
	return store, nil
}

func openRepositoryFromEnv() (*SQLRepository, error) {
	dialectRaw := strings.TrimSpace(strings.ToLower(os.Getenv("DB_DIALECT")))
	if dialectRaw == "" {
		dialectRaw = string(dialectSQLite)
	}
	dialect := DBDialect(dialectRaw)

	var driverName string
	var dsn string
	switch dialect {
	case dialectSQLite:
		driverName = "sqlite"
		path := strings.TrimSpace(os.Getenv("DB_SQLITE_PATH"))
		if path == "" {
			path = filepath.Join("tmp", "black_granary.sqlite")
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("create sqlite directory: %w", err)
		}
		dsn = path
	case dialectPostgres:
		driverName = "pgx"
		dsn = strings.TrimSpace(os.Getenv("DB_POSTGRES_DSN"))
		if dsn == "" {
			dsn = strings.TrimSpace(os.Getenv("DATABASE_URL"))
		}
		if dsn == "" {
			return nil, errors.New("DB_DIALECT=postgres requires DB_POSTGRES_DSN or DATABASE_URL")
		}
	default:
		return nil, fmt.Errorf("unsupported DB_DIALECT %q", dialectRaw)
	}

	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("open %s database: %w", dialect, err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping %s database: %w", dialect, err)
	}

	repo := &SQLRepository{dialect: dialect, db: db}
	if err := repo.applyMigrations(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	log.Printf("database: dialect=%s", dialect)
	return repo, nil
}

func (r *SQLRepository) bind(pos int) string {
	if r.dialect == dialectPostgres {
		return fmt.Sprintf("$%d", pos)
	}
	return "?"
}

func (r *SQLRepository) insertQuery(table string, cols []string) string {
	ph := make([]string, len(cols))
	for i := range cols {
		ph[i] = r.bind(i + 1)
	}
	return fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s)",
		table,
		strings.Join(cols, ", "),
		strings.Join(ph, ", "),
	)
}

func (r *SQLRepository) applyMigrations(ctx context.Context) error {
	create := `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TIMESTAMP NOT NULL
		)
	`
	if _, err := r.db.ExecContext(ctx, create); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	applied := map[string]bool{}
	rows, err := r.db.QueryContext(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return fmt.Errorf("read schema_migrations: %w", err)
	}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return fmt.Errorf("scan schema migration: %w", err)
		}
		applied[v] = true
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate schema migrations: %w", err)
	}
	rows.Close()

	pattern := fmt.Sprintf("migrations/%s/*.sql", r.dialect)
	files, err := fs.Glob(migrationFS, pattern)
	if err != nil {
		return fmt.Errorf("glob migrations: %w", err)
	}
	sort.Strings(files)
	for _, file := range files {
		base := filepath.Base(file)
		if applied[base] {
			continue
		}
		sqlBytes, err := migrationFS.ReadFile(file)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", file, err)
		}
		tx, err := r.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin migration tx %s: %w", file, err)
		}
		if _, err := tx.ExecContext(ctx, string(sqlBytes)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply migration %s: %w", file, err)
		}
		q := r.insertQuery("schema_migrations", []string{"version", "applied_at"})
		if _, err := tx.ExecContext(ctx, q, base, time.Now().UTC()); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %s: %w", file, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", file, err)
		}
	}
	return nil
}

func (store *Store) persistLocked() {
	if store.repo == nil {
		return
	}
	if err := store.repo.Save(context.Background(), store); err != nil {
		log.Printf("persist state failed: %v", err)
	}
}

func (r *SQLRepository) Save(ctx context.Context, store *Store) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin save tx: %w", err)
	}
	if err := r.saveWithTx(ctx, tx, store); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit save tx: %w", err)
	}
	return nil
}

func (r *SQLRepository) saveWithTx(ctx context.Context, tx *sql.Tx, store *Store) error {
	clearTables := []string{
		"world_state", "policy_state", "runtime_state", "players", "institutions", "seats", "contracts",
		"permits", "warrants", "rumors", "evidence", "scry_reports", "intercepts", "loans",
		"obligations", "projects", "active_crisis", "relics", "events", "chat_messages", "diplomatic_messages",
	}
	for _, tbl := range clearTables {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+tbl); err != nil {
			return fmt.Errorf("clear %s: %w", tbl, err)
		}
	}

	now := time.Now().UTC()

	if err := r.insertJSONRow(ctx, tx, "world_state", []string{"id", "payload", "updated_at"}, []any{1, asJSON(store.World), now}); err != nil {
		return err
	}
	if err := r.insertJSONRow(ctx, tx, "policy_state", []string{"id", "payload", "updated_at"}, []any{1, asJSON(store.Policies), now}); err != nil {
		return err
	}

	runtime := runtimeState{
		NextEventID:       store.NextEventID,
		NextContractID:    store.NextContractID,
		NextChatID:        store.NextChatID,
		NextMessageID:     store.NextMessageID,
		NextRumorID:       store.NextRumorID,
		NextEvidenceID:    store.NextEvidenceID,
		NextScryID:        store.NextScryID,
		NextInterceptID:   store.NextInterceptID,
		NextLoanID:        store.NextLoanID,
		NextObligationID:  store.NextObligationID,
		NextProjectID:     store.NextProjectID,
		NextRelicID:       store.NextRelicID,
		LastDailyTickDate: store.LastDailyTickDate,
		LastTickAt:        store.LastTickAt,
		TickEveryNanos:    int64(store.TickEvery),
		TickCount:         store.TickCount,
		LastChatAt:        store.LastChatAt,
		LastMessageAt:     store.LastMessageAt,
		LastActionAt:      store.LastActionAt,
		LastDeliverAt:     store.LastDeliverAt,
		LastInvestigateAt: store.LastInvestigateAt,
		LastSeatActionAt:  store.LastSeatActionAt,
		LastIntelActionAt: store.LastIntelActionAt,
		LastFieldworkAt:   store.LastFieldworkAt,
		DailyActionDate:   store.DailyActionDate,
		DailyHighImpactN:  store.DailyHighImpactN,
		LastCleanupDate:   store.LastCleanupDate,
	}
	if err := r.insertJSONRow(ctx, tx, "runtime_state", []string{"id", "payload", "updated_at"}, []any{1, asJSON(runtime), now}); err != nil {
		return err
	}

	for _, p := range store.Players {
		if err := r.insertJSONRow(ctx, tx, "players",
			[]string{"player_id", "last_seen", "payload", "created_at", "updated_at", "soft_deleted_at", "hard_deleted_at"},
			[]any{p.ID, p.LastSeen, asJSON(p), p.LastSeen, now, p.SoftDeletedAt, p.HardDeletedAt},
		); err != nil {
			return err
		}
	}
	for _, inst := range store.Institutions {
		if err := r.insertJSONRow(ctx, tx, "institutions", []string{"id", "payload", "created_at", "updated_at"}, []any{inst.ID, asJSON(inst), now, now}); err != nil {
			return err
		}
	}
	for _, seat := range store.Seats {
		if err := r.insertJSONRow(ctx, tx, "seats", []string{"id", "payload", "created_at", "updated_at"}, []any{seat.ID, asJSON(seat), now, now}); err != nil {
			return err
		}
	}
	for _, c := range store.Contracts {
		if c.Status != "Issued" && c.Status != "Accepted" {
			continue
		}
		if err := r.insertJSONRow(ctx, tx, "contracts",
			[]string{"contract_id", "status", "owner_player_id", "issued_at_tick", "deadline_ticks", "payload", "created_at", "updated_at", "terminal_at"},
			[]any{c.ID, c.Status, c.OwnerPlayerID, c.IssuedAtTick, c.DeadlineTicks, asJSON(c), now, now, nil},
		); err != nil {
			return err
		}
	}
	for _, permit := range store.Permits {
		expires := store.TickCount + int64(maxInt(0, permit.TicksLeft))
		if err := r.insertJSONRow(ctx, tx, "permits",
			[]string{"player_id", "expires_tick", "payload", "created_at", "updated_at"},
			[]any{permit.PlayerID, expires, asJSON(permit), now, now},
		); err != nil {
			return err
		}
	}
	for _, warrant := range store.Warrants {
		expires := store.TickCount + int64(maxInt(0, warrant.TicksLeft))
		if err := r.insertJSONRow(ctx, tx, "warrants",
			[]string{"player_id", "expires_tick", "payload", "created_at", "updated_at"},
			[]any{warrant.PlayerID, expires, asJSON(warrant), now, now},
		); err != nil {
			return err
		}
	}
	for _, rumor := range store.Rumors {
		expires := store.TickCount + int64(maxInt(0, rumor.Decay))
		if err := r.insertJSONRow(ctx, tx, "rumors", []string{"id", "expires_tick", "payload", "created_at", "updated_at"}, []any{rumor.ID, expires, asJSON(rumor), now, now}); err != nil {
			return err
		}
	}
	for _, ev := range store.Evidence {
		if err := r.insertJSONRow(ctx, tx, "evidence", []string{"id", "expires_tick", "payload", "created_at", "updated_at"}, []any{ev.ID, ev.ExpiryTick, asJSON(ev), now, now}); err != nil {
			return err
		}
	}
	for _, report := range store.ScryReports {
		if err := r.insertJSONRow(ctx, tx, "scry_reports", []string{"id", "owner_player_id", "expires_tick", "payload", "created_at", "updated_at"}, []any{report.ID, report.OwnerPlayerID, report.ExpiryTick, asJSON(report), now, now}); err != nil {
			return err
		}
	}
	for _, intercept := range store.Intercepts {
		if err := r.insertJSONRow(ctx, tx, "intercepts", []string{"id", "owner_player_id", "expires_tick", "payload", "created_at", "updated_at"}, []any{intercept.ID, intercept.OwnerPlayerID, intercept.ExpiryTick, asJSON(intercept), now, now}); err != nil {
			return err
		}
	}
	for _, loan := range store.Loans {
		if err := r.insertJSONRow(ctx, tx, "loans",
			[]string{"id", "status", "due_tick", "terminal_at", "payload", "created_at", "updated_at"},
			[]any{loan.ID, loan.Status, loan.DueTick, nullableTime(loan.TerminalAt), asJSON(loan), now, now},
		); err != nil {
			return err
		}
	}
	for _, ob := range store.Obligations {
		if err := r.insertJSONRow(ctx, tx, "obligations",
			[]string{"id", "status", "due_tick", "terminal_at", "payload", "created_at", "updated_at"},
			[]any{ob.ID, ob.Status, ob.DueTick, nullableTime(ob.TerminalAt), asJSON(ob), now, now},
		); err != nil {
			return err
		}
	}
	for _, proj := range store.Projects {
		if err := r.insertJSONRow(ctx, tx, "projects", []string{"id", "owner_player_id", "payload", "created_at", "updated_at"}, []any{proj.ID, proj.OwnerPlayerID, asJSON(proj), now, now}); err != nil {
			return err
		}
	}
	if store.ActiveCrisis != nil {
		if err := r.insertJSONRow(ctx, tx, "active_crisis", []string{"id", "payload", "updated_at"}, []any{1, asJSON(store.ActiveCrisis), now}); err != nil {
			return err
		}
	}
	for _, relic := range store.Relics {
		if err := r.insertJSONRow(ctx, tx, "relics", []string{"id", "owner_player_id", "payload", "created_at", "updated_at"}, []any{relic.ID, relic.OwnerPlayerID, asJSON(relic), now, now}); err != nil {
			return err
		}
	}

	for _, event := range store.Events {
		if err := r.insertJSONRow(ctx, tx, "events",
			[]string{"id", "at_ts", "day_number", "subphase", "type", "severity", "text", "payload", "created_at"},
			[]any{event.ID, event.At, event.DayNumber, event.Subphase, event.Type, event.Severity, event.Text, asJSON(event), event.At},
		); err != nil {
			return err
		}
	}
	for _, msg := range store.Chat {
		if err := r.insertJSONRow(ctx, tx, "chat_messages",
			[]string{"id", "at_ts", "kind", "from_player_id", "to_player_id", "text", "payload", "created_at"},
			[]any{msg.ID, msg.At, msg.Kind, msg.FromPlayerID, msg.ToPlayerID, msg.Text, asJSON(msg), msg.At},
		); err != nil {
			return err
		}
	}
	for _, msg := range store.Messages {
		if err := r.insertJSONRow(ctx, tx, "diplomatic_messages",
			[]string{"id", "at_ts", "from_player_id", "to_player_id", "subject", "payload", "created_at"},
			[]any{msg.ID, msg.At, msg.FromPlayerID, msg.ToPlayerID, msg.Subject, asJSON(msg), msg.At},
		); err != nil {
			return err
		}
	}

	return nil
}

func (r *SQLRepository) insertJSONRow(ctx context.Context, tx *sql.Tx, table string, cols []string, vals []any) error {
	q := r.insertQuery(table, cols)
	if _, err := tx.ExecContext(ctx, q, vals...); err != nil {
		return fmt.Errorf("insert %s: %w", table, err)
	}
	return nil
}

func asJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func nullableTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}

func (r *SQLRepository) LoadInto(ctx context.Context, store *Store) error {
	var worldRows int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM world_state").Scan(&worldRows); err != nil {
		return fmt.Errorf("count world_state: %w", err)
	}
	if worldRows == 0 {
		if err := r.Save(ctx, store); err != nil {
			return fmt.Errorf("seed initial state: %w", err)
		}
		return nil
	}

	if err := r.loadWorldAndPolicy(ctx, store); err != nil {
		return err
	}
	if err := r.loadRuntime(ctx, store); err != nil {
		return err
	}
	if err := r.loadCollections(ctx, store); err != nil {
		return err
	}
	return nil
}

func (r *SQLRepository) loadWorldAndPolicy(ctx context.Context, store *Store) error {
	var worldPayload string
	err := r.db.QueryRowContext(ctx, "SELECT payload FROM world_state WHERE id = 1").Scan(&worldPayload)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("load world_state: %w", err)
	}
	if worldPayload != "" {
		var world WorldState
		if err := json.Unmarshal([]byte(worldPayload), &world); err != nil {
			return fmt.Errorf("decode world_state: %w", err)
		}
		store.World = world
	}

	var policyPayload string
	err = r.db.QueryRowContext(ctx, "SELECT payload FROM policy_state WHERE id = 1").Scan(&policyPayload)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("load policy_state: %w", err)
	}
	if policyPayload != "" {
		var policy PolicyState
		if err := json.Unmarshal([]byte(policyPayload), &policy); err != nil {
			return fmt.Errorf("decode policy_state: %w", err)
		}
		store.Policies = policy
	}
	return nil
}

func (r *SQLRepository) loadRuntime(ctx context.Context, store *Store) error {
	var payload string
	err := r.db.QueryRowContext(ctx, "SELECT payload FROM runtime_state WHERE id = 1").Scan(&payload)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("load runtime_state: %w", err)
	}
	if payload == "" {
		return nil
	}
	var runtime runtimeState
	if err := json.Unmarshal([]byte(payload), &runtime); err != nil {
		return fmt.Errorf("decode runtime_state: %w", err)
	}
	store.NextEventID = runtime.NextEventID
	store.NextContractID = runtime.NextContractID
	store.NextChatID = runtime.NextChatID
	store.NextMessageID = runtime.NextMessageID
	store.NextRumorID = runtime.NextRumorID
	store.NextEvidenceID = runtime.NextEvidenceID
	store.NextScryID = runtime.NextScryID
	store.NextInterceptID = runtime.NextInterceptID
	store.NextLoanID = runtime.NextLoanID
	store.NextObligationID = runtime.NextObligationID
	store.NextProjectID = runtime.NextProjectID
	store.NextRelicID = runtime.NextRelicID
	store.LastDailyTickDate = runtime.LastDailyTickDate
	store.LastTickAt = runtime.LastTickAt
	if runtime.TickEveryNanos > 0 {
		store.TickEvery = time.Duration(runtime.TickEveryNanos)
	}
	store.TickCount = runtime.TickCount
	store.LastChatAt = runtime.LastChatAt
	store.LastMessageAt = runtime.LastMessageAt
	store.LastActionAt = runtime.LastActionAt
	store.LastDeliverAt = runtime.LastDeliverAt
	store.LastInvestigateAt = runtime.LastInvestigateAt
	store.LastSeatActionAt = runtime.LastSeatActionAt
	store.LastIntelActionAt = runtime.LastIntelActionAt
	store.LastFieldworkAt = runtime.LastFieldworkAt
	store.DailyActionDate = runtime.DailyActionDate
	store.DailyHighImpactN = runtime.DailyHighImpactN
	store.LastCleanupDate = runtime.LastCleanupDate
	ensureRuntimeMaps(store)
	return nil
}

func ensureRuntimeMaps(store *Store) {
	if store.LastChatAt == nil {
		store.LastChatAt = map[string]time.Time{}
	}
	if store.LastMessageAt == nil {
		store.LastMessageAt = map[string]time.Time{}
	}
	if store.LastActionAt == nil {
		store.LastActionAt = map[string]time.Time{}
	}
	if store.LastDeliverAt == nil {
		store.LastDeliverAt = map[string]time.Time{}
	}
	if store.LastInvestigateAt == nil {
		store.LastInvestigateAt = map[string]int64{}
	}
	if store.LastSeatActionAt == nil {
		store.LastSeatActionAt = map[string]int64{}
	}
	if store.LastIntelActionAt == nil {
		store.LastIntelActionAt = map[string]int64{}
	}
	if store.LastFieldworkAt == nil {
		store.LastFieldworkAt = map[string]int64{}
	}
	if store.DailyActionDate == nil {
		store.DailyActionDate = map[string]string{}
	}
	if store.DailyHighImpactN == nil {
		store.DailyHighImpactN = map[string]int{}
	}
}

func (r *SQLRepository) loadCollections(ctx context.Context, store *Store) error {
	store.Players = map[string]*Player{}
	store.Institutions = map[string]*Institution{}
	store.Seats = map[string]*Seat{}
	store.Contracts = map[string]*Contract{}
	store.Permits = map[string]*Permit{}
	store.Warrants = map[string]*Warrant{}
	store.Rumors = map[int64]*Rumor{}
	store.Evidence = map[int64]*Evidence{}
	store.ScryReports = map[int64]*ScryReport{}
	store.Intercepts = map[int64]*InterceptedMessage{}
	store.Loans = map[string]*Loan{}
	store.Obligations = map[string]*Obligation{}
	store.Projects = map[string]*Project{}
	store.Relics = map[int64]*Relic{}
	store.Events = []Event{}
	store.Chat = []ChatMessage{}
	store.Messages = []DiplomaticMessage{}
	store.ActiveCrisis = nil

	if err := loadMapRows(ctx, r.db, "SELECT payload FROM players", func(payload string) error {
		var p Player
		if err := json.Unmarshal([]byte(payload), &p); err != nil {
			return err
		}
		store.Players[p.ID] = &p
		return nil
	}); err != nil {
		return fmt.Errorf("load players: %w", err)
	}
	if err := loadMapRows(ctx, r.db, "SELECT payload FROM institutions", func(payload string) error {
		var inst Institution
		if err := json.Unmarshal([]byte(payload), &inst); err != nil {
			return err
		}
		store.Institutions[inst.ID] = &inst
		return nil
	}); err != nil {
		return fmt.Errorf("load institutions: %w", err)
	}
	if err := loadMapRows(ctx, r.db, "SELECT payload FROM seats", func(payload string) error {
		var seat Seat
		if err := json.Unmarshal([]byte(payload), &seat); err != nil {
			return err
		}
		store.Seats[seat.ID] = &seat
		return nil
	}); err != nil {
		return fmt.Errorf("load seats: %w", err)
	}
	if err := loadMapRows(ctx, r.db, "SELECT payload FROM contracts", func(payload string) error {
		var c Contract
		if err := json.Unmarshal([]byte(payload), &c); err != nil {
			return err
		}
		store.Contracts[c.ID] = &c
		return nil
	}); err != nil {
		return fmt.Errorf("load contracts: %w", err)
	}
	if err := loadMapRows(ctx, r.db, "SELECT payload FROM permits", func(payload string) error {
		var permit Permit
		if err := json.Unmarshal([]byte(payload), &permit); err != nil {
			return err
		}
		store.Permits[permit.PlayerID] = &permit
		return nil
	}); err != nil {
		return fmt.Errorf("load permits: %w", err)
	}
	if err := loadMapRows(ctx, r.db, "SELECT payload FROM warrants", func(payload string) error {
		var warrant Warrant
		if err := json.Unmarshal([]byte(payload), &warrant); err != nil {
			return err
		}
		store.Warrants[warrant.PlayerID] = &warrant
		return nil
	}); err != nil {
		return fmt.Errorf("load warrants: %w", err)
	}
	if err := loadMapRows(ctx, r.db, "SELECT payload FROM rumors", func(payload string) error {
		var rumor Rumor
		if err := json.Unmarshal([]byte(payload), &rumor); err != nil {
			return err
		}
		store.Rumors[rumor.ID] = &rumor
		return nil
	}); err != nil {
		return fmt.Errorf("load rumors: %w", err)
	}
	if err := loadMapRows(ctx, r.db, "SELECT payload FROM evidence", func(payload string) error {
		var ev Evidence
		if err := json.Unmarshal([]byte(payload), &ev); err != nil {
			return err
		}
		store.Evidence[ev.ID] = &ev
		return nil
	}); err != nil {
		return fmt.Errorf("load evidence: %w", err)
	}
	if err := loadMapRows(ctx, r.db, "SELECT payload FROM scry_reports", func(payload string) error {
		var report ScryReport
		if err := json.Unmarshal([]byte(payload), &report); err != nil {
			return err
		}
		store.ScryReports[report.ID] = &report
		return nil
	}); err != nil {
		return fmt.Errorf("load scry_reports: %w", err)
	}
	if err := loadMapRows(ctx, r.db, "SELECT payload FROM intercepts", func(payload string) error {
		var intercept InterceptedMessage
		if err := json.Unmarshal([]byte(payload), &intercept); err != nil {
			return err
		}
		store.Intercepts[intercept.ID] = &intercept
		return nil
	}); err != nil {
		return fmt.Errorf("load intercepts: %w", err)
	}
	if err := loadMapRows(ctx, r.db, "SELECT payload FROM loans", func(payload string) error {
		var loan Loan
		if err := json.Unmarshal([]byte(payload), &loan); err != nil {
			return err
		}
		store.Loans[loan.ID] = &loan
		return nil
	}); err != nil {
		return fmt.Errorf("load loans: %w", err)
	}
	if err := loadMapRows(ctx, r.db, "SELECT payload FROM obligations", func(payload string) error {
		var ob Obligation
		if err := json.Unmarshal([]byte(payload), &ob); err != nil {
			return err
		}
		store.Obligations[ob.ID] = &ob
		return nil
	}); err != nil {
		return fmt.Errorf("load obligations: %w", err)
	}
	if err := loadMapRows(ctx, r.db, "SELECT payload FROM projects", func(payload string) error {
		var proj Project
		if err := json.Unmarshal([]byte(payload), &proj); err != nil {
			return err
		}
		store.Projects[proj.ID] = &proj
		return nil
	}); err != nil {
		return fmt.Errorf("load projects: %w", err)
	}
	if err := loadMapRows(ctx, r.db, "SELECT payload FROM relics", func(payload string) error {
		var relic Relic
		if err := json.Unmarshal([]byte(payload), &relic); err != nil {
			return err
		}
		store.Relics[relic.ID] = &relic
		return nil
	}); err != nil {
		return fmt.Errorf("load relics: %w", err)
	}
	if err := loadMapRows(ctx, r.db, "SELECT payload FROM events ORDER BY id", func(payload string) error {
		var event Event
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			return err
		}
		store.Events = append(store.Events, event)
		return nil
	}); err != nil {
		return fmt.Errorf("load events: %w", err)
	}
	if err := loadMapRows(ctx, r.db, "SELECT payload FROM chat_messages ORDER BY id", func(payload string) error {
		var msg ChatMessage
		if err := json.Unmarshal([]byte(payload), &msg); err != nil {
			return err
		}
		store.Chat = append(store.Chat, msg)
		return nil
	}); err != nil {
		return fmt.Errorf("load chat_messages: %w", err)
	}
	if err := loadMapRows(ctx, r.db, "SELECT payload FROM diplomatic_messages ORDER BY id", func(payload string) error {
		var msg DiplomaticMessage
		if err := json.Unmarshal([]byte(payload), &msg); err != nil {
			return err
		}
		store.Messages = append(store.Messages, msg)
		return nil
	}); err != nil {
		return fmt.Errorf("load diplomatic_messages: %w", err)
	}

	var crisisPayload string
	err := r.db.QueryRowContext(ctx, "SELECT payload FROM active_crisis WHERE id = 1").Scan(&crisisPayload)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("load active_crisis: %w", err)
	}
	if crisisPayload != "" {
		var crisis Crisis
		if err := json.Unmarshal([]byte(crisisPayload), &crisis); err != nil {
			return fmt.Errorf("decode active_crisis: %w", err)
		}
		store.ActiveCrisis = &crisis
	}
	return nil
}

func loadMapRows(ctx context.Context, db *sql.DB, q string, fn func(payload string) error) error {
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return err
		}
		if err := fn(payload); err != nil {
			return err
		}
	}
	return rows.Err()
}

func startCleanupScheduler(store *Store) {
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for now := range ticker.C {
			store.mu.Lock()
			today := now.UTC().Format("2006-01-02")
			if store.LastCleanupDate != today {
				runDailyCleanupLocked(store, now.UTC())
				store.LastCleanupDate = today
				store.persistLocked()
			}
			store.mu.Unlock()
		}
	}()
}

func runDailyCleanupLocked(store *Store, now time.Time) {
	eventsCutoff := now.Add(-14 * 24 * time.Hour)
	chatCutoff := now.Add(-7 * 24 * time.Hour)
	diplCutoff := now.Add(-30 * 24 * time.Hour)

	filteredEvents := make([]Event, 0, len(store.Events))
	for _, e := range store.Events {
		if e.At.After(eventsCutoff) || e.At.Equal(eventsCutoff) {
			filteredEvents = append(filteredEvents, e)
		}
	}
	store.Events = filteredEvents

	filteredChat := make([]ChatMessage, 0, len(store.Chat))
	for _, m := range store.Chat {
		if m.At.After(chatCutoff) || m.At.Equal(chatCutoff) {
			filteredChat = append(filteredChat, m)
		}
	}
	store.Chat = filteredChat

	filteredDipl := make([]DiplomaticMessage, 0, len(store.Messages))
	for _, m := range store.Messages {
		if m.At.After(diplCutoff) || m.At.Equal(diplCutoff) {
			filteredDipl = append(filteredDipl, m)
		}
	}
	store.Messages = filteredDipl

	for id, loan := range store.Loans {
		if (loan.Status == "Repaid" || loan.Status == "Defaulted" || loan.Status == "Cancelled") && !loan.TerminalAt.IsZero() {
			if loan.TerminalAt.Before(now.Add(-30 * 24 * time.Hour)) {
				delete(store.Loans, id)
			}
		}
	}

	for id, ob := range store.Obligations {
		if ob.TerminalAt.IsZero() {
			continue
		}
		switch ob.Status {
		case "Settled", "Forgiven":
			if ob.TerminalAt.Before(now.Add(-30 * 24 * time.Hour)) {
				delete(store.Obligations, id)
			}
		case "Overdue":
			if ob.TerminalAt.Before(now.Add(-60 * 24 * time.Hour)) {
				delete(store.Obligations, id)
			}
		}
	}

	for id, p := range store.Players {
		inactiveFor := now.Sub(p.LastSeen)
		if inactiveFor >= 90*24*time.Hour && p.SoftDeletedAt.IsZero() {
			p.SoftDeletedAt = now
		}
		if inactiveFor >= 180*24*time.Hour {
			p.HardDeletedAt = now
			p.Name = fmt.Sprintf("Former Citizen %s", shortID(p.ID))
			p.Gold = 0
			p.Grain = 0
			p.Rep = 0
			p.Heat = 0
			p.Rumors = 0
			delete(store.ToastByPlayer, id)
		}
	}
}

func shortID(id string) string {
	if len(id) <= 6 {
		return id
	}
	return id[:6]
}
