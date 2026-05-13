package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"
	_ "modernc.org/sqlite"
)

type Database struct {
	db *sql.DB
}

var globalDB *Database

func InitializeDatabase(configDir string) error {
	// Ensure config directory exists
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	dbPath := filepath.Join(configDir, "database.sqlite")
	log.Debug().Str("path", dbPath).Msg("Initializing SQLite database")

	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=ON")
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	// SQLite file database works best with a single writer connection
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(4)

	globalDB = &Database{db: db}

	if err := globalDB.createTables(); err != nil {
		return fmt.Errorf("failed to create tables: %w", err)
	}

	log.Debug().Msg("SQLite database initialized successfully")
	return nil
}

// GetDatabase returns the global database instance.
// Panics if InitializeDatabase was not called first.
func GetDatabase() *Database {
	if globalDB == nil {
		panic("database not initialized; call InitializeDatabase first")
	}
	return globalDB
}

func (db *Database) createTables() error {
	// Core tables — never modified after creation; schema changes go through migrations below.
	tables := []string{
		// Schema migrations — tracks which migrations have been applied.
		`CREATE TABLE IF NOT EXISTS schema_migrations (
			version    INTEGER PRIMARY KEY,
			applied_at DATETIME NOT NULL DEFAULT (datetime('now'))
		)`,

		// Key-value application configuration.
		`CREATE TABLE IF NOT EXISTS config (
			key        TEXT PRIMARY KEY,
			value      TEXT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT (datetime('now')),
			updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
		)`,

		// Provider instances — root entity for every configured AI provider.
		`CREATE TABLE IF NOT EXISTS provider_instances (
			instance_id TEXT    PRIMARY KEY,
			provider_id TEXT    NOT NULL,
			name        TEXT    NOT NULL,
			subtitle    TEXT    NOT NULL DEFAULT '',
			priority    INTEGER NOT NULL DEFAULT 0,
			activated   INTEGER NOT NULL DEFAULT 0 CHECK (activated IN (0, 1)),
			created_at  DATETIME NOT NULL DEFAULT (datetime('now')),
			updated_at  DATETIME NOT NULL DEFAULT (datetime('now'))
		)`,

		// Credentials for each provider instance (one row per instance).
		// provider_id is intentionally omitted — join provider_instances for that.
		`CREATE TABLE IF NOT EXISTS tokens (
			instance_id TEXT PRIMARY KEY,
			token_data  TEXT NOT NULL,
			created_at  DATETIME NOT NULL DEFAULT (datetime('now')),
			updated_at  DATETIME NOT NULL DEFAULT (datetime('now')),
			FOREIGN KEY (instance_id) REFERENCES provider_instances (instance_id) ON DELETE CASCADE
		)`,

		// Per-model enabled/disabled overrides for a provider instance.
		`CREATE TABLE IF NOT EXISTS provider_model_states (
			instance_id TEXT    NOT NULL,
			model_id    TEXT    NOT NULL,
			enabled     INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
			created_at  DATETIME NOT NULL DEFAULT (datetime('now')),
			updated_at  DATETIME NOT NULL DEFAULT (datetime('now')),
			PRIMARY KEY (instance_id, model_id),
			FOREIGN KEY (instance_id) REFERENCES provider_instances (instance_id) ON DELETE CASCADE
		)`,

		// Provider-level configuration blob (one row per instance).
		`CREATE TABLE IF NOT EXISTS provider_configs (
			instance_id TEXT PRIMARY KEY,
			config_data TEXT NOT NULL,
			created_at  DATETIME NOT NULL DEFAULT (datetime('now')),
			updated_at  DATETIME NOT NULL DEFAULT (datetime('now')),
			FOREIGN KEY (instance_id) REFERENCES provider_instances (instance_id) ON DELETE CASCADE
		)`,

		// Per-model configuration and version tracking.
		// version is INTEGER so numeric ordering works correctly.
		`CREATE TABLE IF NOT EXISTS model_configs (
			instance_id TEXT    NOT NULL,
			model_id    TEXT    NOT NULL,
			version     INTEGER NOT NULL DEFAULT 1,
			config_data TEXT    NOT NULL DEFAULT '{}',
			created_at  DATETIME NOT NULL DEFAULT (datetime('now')),
			updated_at  DATETIME NOT NULL DEFAULT (datetime('now')),
			PRIMARY KEY (instance_id, model_id),
			FOREIGN KEY (instance_id) REFERENCES provider_instances (instance_id) ON DELETE CASCADE
		)`,

		// Cached model list returned by each provider's /models endpoint.
		`CREATE TABLE IF NOT EXISTS provider_models_cache (
			instance_id TEXT PRIMARY KEY,
			models_data TEXT    NOT NULL,
			cached_at   DATETIME NOT NULL DEFAULT (datetime('now')),
			updated_at  DATETIME NOT NULL DEFAULT (datetime('now')),
			FOREIGN KEY (instance_id) REFERENCES provider_instances (instance_id) ON DELETE CASCADE
		)`,

		// Chat sessions.
		`CREATE TABLE IF NOT EXISTS chat_sessions (
			session_id    TEXT PRIMARY KEY,
			title         TEXT NOT NULL,
			model_id      TEXT NOT NULL,
			api_shape     TEXT NOT NULL DEFAULT 'anthropic',
			agent_backend TEXT NOT NULL DEFAULT 'omnicode',
			created_at    DATETIME NOT NULL DEFAULT (datetime('now')),
			updated_at    DATETIME NOT NULL DEFAULT (datetime('now'))
		)`,

		// Chat messages — role is enforced at the DB level.
		`CREATE TABLE IF NOT EXISTS chat_messages (
			message_id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			role       TEXT NOT NULL CHECK (role IN ('user', 'assistant', 'system')),
			content    TEXT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT (datetime('now')),
			FOREIGN KEY (session_id) REFERENCES chat_sessions (session_id) ON DELETE CASCADE
		)`,

		// Virtual (load-balanced) models.
		`CREATE TABLE IF NOT EXISTS virtual_models (
			virtual_model_id TEXT    PRIMARY KEY,
			name             TEXT    NOT NULL,
			description      TEXT    NOT NULL DEFAULT '',
			api_shape        TEXT    NOT NULL DEFAULT 'openai',
			lb_strategy      TEXT    NOT NULL DEFAULT 'round-robin'
			                         CHECK (lb_strategy IN ('round-robin','random','priority','weighted')),
			enabled          INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
			created_at       DATETIME NOT NULL DEFAULT (datetime('now')),
			updated_at       DATETIME NOT NULL DEFAULT (datetime('now'))
		)`,

		// Upstream provider+model entries for each virtual model.
		`CREATE TABLE IF NOT EXISTS virtual_model_upstreams (
			id               INTEGER PRIMARY KEY AUTOINCREMENT,
			virtual_model_id TEXT    NOT NULL,
			provider_id      TEXT    NOT NULL DEFAULT '',
			model_id         TEXT    NOT NULL,
			weight           INTEGER NOT NULL DEFAULT 1,
			priority         INTEGER NOT NULL DEFAULT 0,
			created_at       DATETIME NOT NULL DEFAULT (datetime('now')),
			updated_at       DATETIME NOT NULL DEFAULT (datetime('now')),
			FOREIGN KEY (virtual_model_id) REFERENCES virtual_models (virtual_model_id) ON DELETE CASCADE
		)`,

		// Per-request metering log — one row per completed API call.
		`CREATE TABLE IF NOT EXISTS request_logs (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			request_id    TEXT    NOT NULL DEFAULT '',
			model_id      TEXT    NOT NULL DEFAULT '',
			model_used    TEXT    NOT NULL DEFAULT '',
			provider_id   TEXT    NOT NULL DEFAULT '',
			client        TEXT    NOT NULL DEFAULT 'unknown',
			api_shape     TEXT    NOT NULL DEFAULT 'openai',
			input_tokens  INTEGER NOT NULL DEFAULT 0,
			output_tokens INTEGER NOT NULL DEFAULT 0,
			total_tokens  INTEGER NOT NULL DEFAULT 0,
			latency_ms    INTEGER NOT NULL DEFAULT 0,
			is_stream     INTEGER NOT NULL DEFAULT 0,
			status_code   INTEGER NOT NULL DEFAULT 200,
			error_message TEXT    NOT NULL DEFAULT '',
			created_at    DATETIME NOT NULL DEFAULT (datetime('now'))
		)`,
	}

	for _, q := range tables {
		if _, err := db.db.Exec(q); err != nil {
			return fmt.Errorf("failed to create table: %w", err)
		}
	}

	// Indexes — IF NOT EXISTS makes these idempotent on every startup.
	// Note: idx_provider_model_states_instance_id is intentionally absent —
	// the composite PK (instance_id, model_id) already covers instance_id lookups.
	// idx_provider_models_cache_cached_at is intentionally absent — TTL checks
	// always go through the PK; there is no bulk-expiry sweep.
	indexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_provider_instances_provider_id   ON provider_instances (provider_id)`,
		`CREATE INDEX IF NOT EXISTS idx_provider_instances_priority       ON provider_instances (priority)`,
		`CREATE INDEX IF NOT EXISTS idx_provider_instances_activated      ON provider_instances (activated)`,
		`CREATE INDEX IF NOT EXISTS idx_provider_model_states_enabled     ON provider_model_states (enabled)`,
		`CREATE INDEX IF NOT EXISTS idx_model_configs_instance_id         ON model_configs (instance_id)`,
		`CREATE INDEX IF NOT EXISTS idx_model_configs_model_id            ON model_configs (model_id)`,
		`CREATE INDEX IF NOT EXISTS idx_chat_messages_session_id          ON chat_messages (session_id)`,
		`CREATE INDEX IF NOT EXISTS idx_chat_sessions_updated_at          ON chat_sessions (updated_at)`,
		`CREATE INDEX IF NOT EXISTS idx_chat_messages_session_created_at  ON chat_messages (session_id, created_at, message_id)`,
		`CREATE INDEX IF NOT EXISTS idx_virtual_model_upstreams_virtual_model_id ON virtual_model_upstreams (virtual_model_id)`,
		`CREATE INDEX IF NOT EXISTS idx_virtual_model_upstreams_ordering  ON virtual_model_upstreams (virtual_model_id, priority, id)`,
		`CREATE INDEX IF NOT EXISTS idx_request_logs_created_at           ON request_logs (created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_request_logs_model_created_at     ON request_logs (model_id, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_request_logs_provider_created_at  ON request_logs (provider_id, created_at)`,
	}

	for _, q := range indexes {
		if _, err := db.db.Exec(q); err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}

	if err := db.applyMigrations(); err != nil {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}

	log.Debug().Msg("Database tables created successfully")
	return nil
}

// migration represents a single, idempotent schema change.
type migration struct {
	version    int
	statements []string
}

// migrations is the ordered list of all schema migrations.
// Never edit an existing entry — always append a new one.
var migrations = []migration{
	// v1: add subtitle to provider_instances (backfill for pre-subtitle databases).
	{1, []string{`ALTER TABLE provider_instances ADD COLUMN subtitle TEXT NOT NULL DEFAULT ''`}},
	// v2: add provider_id to virtual_model_upstreams (backfill for pre-provider_id databases).
	{2, []string{`ALTER TABLE virtual_model_upstreams ADD COLUMN provider_id TEXT NOT NULL DEFAULT ''`} },
	// v3: add provider_id column to tokens for existing rows that pre-date its removal
	//     from the schema (kept for backward-compat reads; new code ignores it).
	//     No-op on fresh databases where the column was never added.
	{3, []string{`ALTER TABLE tokens ADD COLUMN provider_id TEXT NOT NULL DEFAULT ''`} },
	// v4: add updated_at to provider_models_cache using a SQLite-safe backfill path.
	{4, []string{
		`ALTER TABLE provider_models_cache ADD COLUMN updated_at DATETIME`,
		`UPDATE provider_models_cache SET updated_at = cached_at WHERE updated_at IS NULL`,
	}},
	// v5: rebuild the tokens table to remove the legacy provider_id column that some
	// older databases have as TEXT NOT NULL without a DEFAULT value.  The column was
	// dropped from the schema but may still exist in databases created before that
	// change.  Migration v3 attempted to add it back with DEFAULT '', but was silently
	// skipped when the column already existed with the broken NOT NULL constraint,
	// causing every INSERT into tokens to fail with a constraint error.
	// We use SQLite's table-rebuild idiom and drop any legacy index first.
	{5, []string{
		`DROP INDEX IF EXISTS idx_tokens_provider_id`,
		`CREATE TABLE IF NOT EXISTS tokens_v5 (
			instance_id TEXT PRIMARY KEY,
			token_data  TEXT NOT NULL,
			created_at  DATETIME NOT NULL DEFAULT (datetime('now')),
			updated_at  DATETIME NOT NULL DEFAULT (datetime('now')),
			FOREIGN KEY (instance_id) REFERENCES provider_instances (instance_id) ON DELETE CASCADE
		)`,
		`INSERT OR IGNORE INTO tokens_v5 (instance_id, token_data, created_at, updated_at)
			SELECT instance_id, token_data, created_at, updated_at FROM tokens`,
		`DROP TABLE tokens`,
		`ALTER TABLE tokens_v5 RENAME TO tokens`,
	}},
	// v6: add updated_at to virtual_model_upstreams and create indexes that match
	// current query patterns for sessions, messages, and upstream ordering.
	{6, []string{
		`ALTER TABLE virtual_model_upstreams ADD COLUMN updated_at DATETIME`,
		`UPDATE virtual_model_upstreams SET updated_at = created_at WHERE updated_at IS NULL`,
		`CREATE INDEX IF NOT EXISTS idx_chat_sessions_updated_at ON chat_sessions (updated_at)`,
		`CREATE INDEX IF NOT EXISTS idx_chat_messages_session_created_at ON chat_messages (session_id, created_at, message_id)`,
		`CREATE INDEX IF NOT EXISTS idx_virtual_model_upstreams_ordering ON virtual_model_upstreams (virtual_model_id, priority, id)`,
	}},
	// v7: add request_logs table for per-request metering. The table is also
	// created in createTables for fresh databases; this migration handles
	// existing databases that pre-date it.
	{7, []string{
		`CREATE TABLE IF NOT EXISTS request_logs (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			request_id    TEXT    NOT NULL DEFAULT '',
			model_id      TEXT    NOT NULL DEFAULT '',
			model_used    TEXT    NOT NULL DEFAULT '',
			provider_id   TEXT    NOT NULL DEFAULT '',
			client        TEXT    NOT NULL DEFAULT 'unknown',
			api_shape     TEXT    NOT NULL DEFAULT 'openai',
			input_tokens  INTEGER NOT NULL DEFAULT 0,
			output_tokens INTEGER NOT NULL DEFAULT 0,
			total_tokens  INTEGER NOT NULL DEFAULT 0,
			latency_ms    INTEGER NOT NULL DEFAULT 0,
			is_stream     INTEGER NOT NULL DEFAULT 0,
			status_code   INTEGER NOT NULL DEFAULT 200,
			error_message TEXT    NOT NULL DEFAULT '',
			created_at    DATETIME NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_request_logs_created_at          ON request_logs (created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_request_logs_model_created_at    ON request_logs (model_id, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_request_logs_provider_created_at ON request_logs (provider_id, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_request_logs_client_created_at   ON request_logs (client, created_at)`,
	}},
	// v8: add client dimension to request_logs for per-client metering breakdowns.
	{8, []string{
		`ALTER TABLE request_logs ADD COLUMN client TEXT NOT NULL DEFAULT 'unknown'`,
		`CREATE INDEX IF NOT EXISTS idx_request_logs_client_created_at ON request_logs (client, created_at)`,
	}},
	// v9: access tokens table for proxy authentication.
	{9, []string{
		`CREATE TABLE IF NOT EXISTS access_tokens (
			id          TEXT PRIMARY KEY,
			name        TEXT NOT NULL,
			token_hash  TEXT NOT NULL UNIQUE,
			prefix      TEXT NOT NULL,
			created_at  DATETIME DEFAULT (datetime('now')),
			expires_at  DATETIME,
			last_used_at DATETIME,
			enabled     INTEGER NOT NULL DEFAULT 1
		)`,
		`CREATE INDEX IF NOT EXISTS idx_access_tokens_token_hash ON access_tokens (token_hash)`,
		`CREATE INDEX IF NOT EXISTS idx_access_tokens_enabled ON access_tokens (enabled)`,
	}},
	// v10: persist raw token for admin reveal-after-refresh behavior.
	{10, []string{
		`ALTER TABLE access_tokens ADD COLUMN token_plaintext TEXT NOT NULL DEFAULT ''`,
	}},
	// v11: migrate existing Alibaba instances backed by ModelScope endpoints
	// to the new "alibaba-modelscope" provider type.  Only rows whose
	// provider_configs.config_data contains "modelscope.cn" are affected;
	// DashScope instances remain unchanged.
	{11, []string{
		`UPDATE provider_instances SET provider_id = 'alibaba-modelscope'
		 WHERE provider_id = 'alibaba'
		 AND instance_id IN (
		     SELECT pc.instance_id FROM provider_configs pc
		     WHERE pc.config_data LIKE '%modelscope.cn%'
		 )`,
	}},
	// v12: persist selected chat agent backend per session.
	{12, []string{
		`ALTER TABLE chat_sessions ADD COLUMN agent_backend TEXT NOT NULL DEFAULT 'google-adk'`,
	}},
	// v13: normalize OmniCode agent sessions onto omnicode and anthropic /v1/messages.
	{13, []string{
		`UPDATE chat_sessions SET api_shape = 'anthropic' WHERE api_shape IS NULL OR api_shape = '' OR api_shape != 'anthropic'`,
		`UPDATE chat_sessions SET agent_backend = 'omnicode' WHERE agent_backend IS NULL OR agent_backend = '' OR agent_backend != 'omnicode'`,
	}},
}

// applyMigrations runs any migrations that have not yet been applied.
func (db *Database) applyMigrations() error {
	for _, m := range migrations {
		var count int
		err := db.db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, m.version).Scan(&count)
		if err != nil {
			return fmt.Errorf("migration %d: failed to check status: %w", m.version, err)
		}
		if count > 0 {
			continue // already applied
		}

		for _, statement := range m.statements {
			if _, err := db.db.Exec(statement); err != nil {
				// "duplicate column name" means the column exists — treat as already applied.
				if strings.Contains(err.Error(), "duplicate column name") {
					log.Debug().Int("version", m.version).Msg("Migration already applied (column exists), continuing")
					continue
				}
				return fmt.Errorf("migration %d: %w", m.version, err)
			}
		}

		if _, err := db.db.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, m.version); err != nil {
			return fmt.Errorf("migration %d: failed to record: %w", m.version, err)
		}
		log.Debug().Int("version", m.version).Msg("Applied schema migration")
	}
	return nil
}

func (db *Database) Close() error {
	if db.db != nil {
		log.Debug().Msg("Database connection closed")
		return db.db.Close()
	}
	return nil
}
