package database

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "database-test-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmpDir)

	if err := InitializeDatabase(tmpDir); err != nil {
		panic(err)
	}

	os.Exit(m.Run())
}

func TestParseTimeSupportsKnownFormats(t *testing.T) {
	tests := []string{
		"2026-04-12T10:11:12Z",
		"2026-04-12T10:11:12.123456789Z",
		"2026-04-12 10:11:12",
	}

	for _, input := range tests {
		if got := parseTime(input); got.IsZero() {
			t.Fatalf("expected %q to parse", input)
		}
	}
	if got := parseTime("not-a-time"); !got.IsZero() {
		t.Fatal("expected invalid time to return zero value")
	}
}

func TestConfigStoreCRUD(t *testing.T) {
	store := NewConfigStore()
	if err := store.Set("theme", "dark"); err != nil {
		t.Fatalf("set failed: %v", err)
	}
	value, err := store.Get("theme")
	if err != nil || value != "dark" {
		t.Fatalf("expected dark, got %q err=%v", value, err)
	}
	all, err := store.GetAll()
	if err != nil || all["theme"] != "dark" {
		t.Fatalf("expected theme in GetAll, got %#v err=%v", all, err)
	}
	if err := store.Delete("theme"); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	value, err = store.Get("theme")
	if err != nil || value != "" {
		t.Fatalf("expected deleted key to return empty string, got %q err=%v", value, err)
	}
}

func TestProviderInstanceStoreCRUD(t *testing.T) {
	store := NewProviderInstanceStore()
	record := &ProviderInstanceRecord{InstanceID: "provider-1", ProviderID: "mock", Name: "Mock", Priority: 7, Activated: true}
	if err := store.Save(record); err != nil {
		t.Fatalf("save failed: %v", err)
	}
	got, err := store.Get("provider-1")
	if err != nil || got == nil {
		t.Fatalf("get failed: %#v err=%v", got, err)
	}
	if got.Priority != 7 || !got.Activated {
		t.Fatalf("unexpected record: %#v", got)
	}
	if err := store.SetActivation("provider-1", false); err != nil {
		t.Fatalf("set activation failed: %v", err)
	}
	got, _ = store.Get("provider-1")
	if got.Activated {
		t.Fatal("expected provider to be deactivated")
	}
	all, err := store.GetAll()
	if err != nil || len(all) == 0 {
		t.Fatalf("expected provider in GetAll, got %#v err=%v", all, err)
	}
	if err := store.Delete("provider-1"); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	got, err = store.Get("provider-1")
	if err != nil || got != nil {
		t.Fatalf("expected deleted provider to be nil, got %#v err=%v", got, err)
	}
}

func TestTokenStoreCRUD(t *testing.T) {
	// Insert a provider_instances row so the FK and JOIN in GetAllByProvider work.
	instanceStore := NewProviderInstanceStore()
	if err := instanceStore.Save(&ProviderInstanceRecord{
		InstanceID: "provider-1",
		ProviderID: "mock",
		Name:       "Mock Provider",
	}); err != nil {
		t.Fatalf("setup provider instance: %v", err)
	}

	store := NewTokenStore()
	if err := store.Save("provider-1", map[string]any{"token": "abc", "expires": 123}); err != nil {
		t.Fatalf("save failed: %v", err)
	}
	got, err := store.Get("provider-1")
	if err != nil || got == nil {
		t.Fatalf("get failed: %#v err=%v", got, err)
	}
	all, err := store.GetAllByProvider("mock")
	if err != nil || len(all) == 0 {
		t.Fatalf("expected token in GetAllByProvider, got %#v err=%v", all, err)
	}
	if err := store.Delete("provider-1"); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
}

func TestChatStoreCRUD(t *testing.T) {
	store := NewChatStore()
	if err := store.CreateSession("session-1", "Title", "model-1", "openai", "agent-sdk-go"); err != nil {
		t.Fatalf("create session failed: %v", err)
	}
	if err := store.AddMessage("msg-1", "session-1", "user", "hello"); err != nil {
		t.Fatalf("add message failed: %v", err)
	}
	session, err := store.GetSession("session-1")
	if err != nil || session == nil {
		t.Fatalf("get session failed: %#v err=%v", session, err)
	}
	if err := store.UpdateSessionTitle("session-1", "Updated"); err != nil {
		t.Fatalf("update title failed: %v", err)
	}
	if err := store.TouchSession("session-1"); err != nil {
		t.Fatalf("touch session failed: %v", err)
	}
	messages, err := store.GetMessages("session-1")
	if err != nil || len(messages) != 1 {
		t.Fatalf("expected one message, got %#v err=%v", messages, err)
	}
	sessions, err := store.ListSessions()
	if err != nil || len(sessions) == 0 {
		t.Fatalf("expected sessions, got %#v err=%v", sessions, err)
	}
	if err := store.DeleteSession("session-1"); err != nil {
		t.Fatalf("delete session failed: %v", err)
	}
}

func TestChatStoreOrdersMessagesDeterministically(t *testing.T) {
	store := NewChatStore()
	if err := store.CreateSession("session-order", "Title", "model-1", "openai", "agent-sdk-go"); err != nil {
		t.Fatalf("create session failed: %v", err)
	}
	for _, msgID := range []string{"msg-b", "msg-a", "msg-c"} {
		if err := store.AddMessage(msgID, "session-order", "user", msgID); err != nil {
			t.Fatalf("add message %s failed: %v", msgID, err)
		}
	}
	messages, err := store.GetMessages("session-order")
	if err != nil {
		t.Fatalf("get messages failed: %v", err)
	}
	if len(messages) != 3 {
		t.Fatalf("expected three messages, got %#v", messages)
	}
	if messages[0].MessageID != "msg-a" || messages[1].MessageID != "msg-b" || messages[2].MessageID != "msg-c" {
		t.Fatalf("expected deterministic message ordering, got %#v", messages)
	}
}

func TestChatStoreOrdersSessionsDeterministically(t *testing.T) {
	store := NewChatStore()
	for _, sessionID := range []string{"session-b", "session-a", "session-c"} {
		if err := store.CreateSession(sessionID, sessionID, "model-1", "openai", "agent-sdk-go"); err != nil {
			t.Fatalf("create session %s failed: %v", sessionID, err)
		}
	}
	if _, err := GetDatabase().db.Exec(`UPDATE chat_sessions SET updated_at = '2026-04-28 10:00:00' WHERE session_id IN ('session-a', 'session-b', 'session-c')`); err != nil {
		t.Fatalf("normalize updated_at failed: %v", err)
	}
	sessions, err := store.ListSessions()
	if err != nil {
		t.Fatalf("list sessions failed: %v", err)
	}
	var filtered []string
	for _, session := range sessions {
		if session.SessionID == "session-a" || session.SessionID == "session-b" || session.SessionID == "session-c" {
			filtered = append(filtered, session.SessionID)
		}
	}
	if len(filtered) != 3 {
		t.Fatalf("expected three matching sessions, got %#v", filtered)
	}
	if filtered[0] != "session-a" || filtered[1] != "session-b" || filtered[2] != "session-c" {
		t.Fatalf("expected deterministic session ordering, got %#v", filtered)
	}
}

func TestModelStateStoreCRUD(t *testing.T) {
	instanceStore := NewProviderInstanceStore()
	if err := instanceStore.Save(&ProviderInstanceRecord{InstanceID: "provider-ms", ProviderID: "mock", Name: "Mock"}); err != nil {
		t.Fatalf("save provider failed: %v", err)
	}
	store := NewModelStateStore()
	if err := store.SetEnabled("provider-ms", "model-1", true); err != nil {
		t.Fatalf("set enabled failed: %v", err)
	}
	got, err := store.Get("provider-ms", "model-1")
	if err != nil || got == nil || !got.Enabled {
		t.Fatalf("unexpected model state: %#v err=%v", got, err)
	}
	all, err := store.GetAllByInstance("provider-ms")
	if err != nil || len(all) != 1 {
		t.Fatalf("expected one model state, got %#v err=%v", all, err)
	}
	if err := store.Delete("provider-ms", "model-1"); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
}

func TestModelConfigStoreVersionRoundTrip(t *testing.T) {
	instanceStore := NewProviderInstanceStore()
	if err := instanceStore.Save(&ProviderInstanceRecord{InstanceID: "provider-mc", ProviderID: "mock", Name: "Mock"}); err != nil {
		t.Fatalf("save provider failed: %v", err)
	}
	store := NewModelConfigStore()
	if err := store.SetVersion("provider-mc", "model-1", "7"); err != nil {
		t.Fatalf("set version failed: %v", err)
	}
	got, err := store.Get("provider-mc", "model-1")
	if err != nil || got == nil {
		t.Fatalf("get model config failed: %#v err=%v", got, err)
	}
	if got.Version != 7 {
		t.Fatalf("expected version 7, got %#v", got)
	}
}

func TestVirtualModelStoresCRUD(t *testing.T) {
	vmStore := NewVirtualModelStore()
	upstreamStore := NewVirtualModelUpstreamStore()

	record := &VirtualModelRecord{
		VirtualModelID: "vm-1",
		Name:           "VM 1",
		Description:    "test",
		APIShape:       "openai",
		LbStrategy:     LbStrategyWeighted,
		Enabled:        true,
	}
	if err := vmStore.Create(record); err != nil {
		t.Fatalf("create virtual model failed: %v", err)
	}
	got, err := vmStore.Get("vm-1")
	if err != nil || got == nil || got.Name != "VM 1" {
		t.Fatalf("unexpected virtual model: %#v err=%v", got, err)
	}
	gotCaseInsensitive, err := vmStore.Get("VM-1")
	if err != nil || gotCaseInsensitive == nil || gotCaseInsensitive.VirtualModelID != "vm-1" {
		t.Fatalf("expected case-insensitive virtual model lookup, got %#v err=%v", gotCaseInsensitive, err)
	}
	caseVariant := &VirtualModelRecord{
		VirtualModelID: "VM-1",
		Name:           "VM 1 exact case",
		Description:    "test",
		APIShape:       "openai",
		LbStrategy:     LbStrategyWeighted,
		Enabled:        true,
	}
	if err := vmStore.Create(caseVariant); err != nil {
		t.Fatalf("create case variant virtual model failed: %v", err)
	}
	gotExactCase, err := vmStore.Get("VM-1")
	if err != nil || gotExactCase == nil || gotExactCase.VirtualModelID != "VM-1" {
		t.Fatalf("expected exact case virtual model lookup, got %#v err=%v", gotExactCase, err)
	}
	record.Name = "VM 1 updated"
	record.Enabled = false
	if err := vmStore.Update(record); err != nil {
		t.Fatalf("update virtual model failed: %v", err)
	}
	all, err := vmStore.GetAll()
	if err != nil || len(all) == 0 {
		t.Fatalf("expected virtual models, got %#v err=%v", all, err)
	}
	upstreams := []VirtualModelUpstreamRecord{
		{ProviderID: "provider-a", ModelID: "model-a", Weight: 2, Priority: 1},
		{ProviderID: "provider-b", ModelID: "model-b", Weight: 1, Priority: 2},
	}
	if err := upstreamStore.SetForVModel("vm-1", upstreams); err != nil {
		t.Fatalf("set upstreams failed: %v", err)
	}
	gotUpstreams, err := upstreamStore.GetForVModel("vm-1")
	if err != nil || len(gotUpstreams) != 2 {
		t.Fatalf("expected two upstreams, got %#v err=%v", gotUpstreams, err)
	}
	if gotUpstreams[0].ProviderID != "provider-a" || gotUpstreams[1].ProviderID != "provider-b" {
		t.Fatalf("unexpected upstream order: %#v", gotUpstreams)
	}
	if gotUpstreams[0].UpdatedAt.IsZero() || gotUpstreams[1].UpdatedAt.IsZero() {
		t.Fatalf("expected upstream updated_at timestamps, got %#v", gotUpstreams)
	}
	if err := vmStore.Delete("VM-1"); err != nil {
		t.Fatalf("delete case variant virtual model failed: %v", err)
	}
	if err := vmStore.Delete("vm-1"); err != nil {
		t.Fatalf("delete virtual model failed: %v", err)
	}
}

func TestCreateTablesIsIdempotentForBackfill(t *testing.T) {
	db := GetDatabase()
	if err := db.createTables(); err != nil {
		t.Fatalf("first createTables call failed: %v", err)
	}
	if err := db.createTables(); err != nil {
		t.Fatalf("second createTables call failed: %v", err)
	}
}

func TestCreateTablesMigratesLegacyProviderModelsCache(t *testing.T) {
	tmpDir := t.TempDir()
	rawDB, err := sql.Open("sqlite", filepath.Join(tmpDir, "legacy.sqlite"))
	if err != nil {
		t.Fatalf("open legacy database: %v", err)
	}
	defer rawDB.Close()

	legacy := &Database{db: rawDB}
	statements := []string{
		`CREATE TABLE schema_migrations (
			version    INTEGER PRIMARY KEY,
			applied_at DATETIME NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE provider_instances (
			instance_id TEXT PRIMARY KEY,
			provider_id TEXT NOT NULL,
			name        TEXT NOT NULL,
			priority    INTEGER NOT NULL DEFAULT 0,
			activated   INTEGER NOT NULL DEFAULT 0 CHECK (activated IN (0, 1)),
			created_at  DATETIME NOT NULL DEFAULT (datetime('now')),
			updated_at  DATETIME NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE tokens (
			instance_id TEXT PRIMARY KEY,
			token_data  TEXT NOT NULL,
			created_at  DATETIME NOT NULL DEFAULT (datetime('now')),
			updated_at  DATETIME NOT NULL DEFAULT (datetime('now')),
			FOREIGN KEY (instance_id) REFERENCES provider_instances (instance_id) ON DELETE CASCADE
		)`,
		`CREATE TABLE virtual_models (
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
		`CREATE TABLE virtual_model_upstreams (
			id               INTEGER PRIMARY KEY AUTOINCREMENT,
			virtual_model_id TEXT    NOT NULL,
			model_id         TEXT    NOT NULL,
			weight           INTEGER NOT NULL DEFAULT 1,
			priority         INTEGER NOT NULL DEFAULT 0,
			created_at       DATETIME NOT NULL DEFAULT (datetime('now')),
			FOREIGN KEY (virtual_model_id) REFERENCES virtual_models (virtual_model_id) ON DELETE CASCADE
		)`,
		`CREATE TABLE provider_models_cache (
			instance_id TEXT PRIMARY KEY,
			models_data TEXT    NOT NULL,
			cached_at   DATETIME NOT NULL DEFAULT (datetime('now')),
			FOREIGN KEY (instance_id) REFERENCES provider_instances (instance_id) ON DELETE CASCADE
		)`,
		`INSERT INTO schema_migrations (version) VALUES (1), (2), (3)`,
		`INSERT INTO provider_instances (instance_id, provider_id, name) VALUES ('legacy-provider', 'mock', 'Legacy Provider')`,
		`INSERT INTO provider_models_cache (instance_id, models_data, cached_at) VALUES ('legacy-provider', '{}', '2026-04-25 12:34:56')`,
	}

	for _, statement := range statements {
		if _, err := rawDB.Exec(statement); err != nil {
			t.Fatalf("seed legacy schema: %v", err)
		}
	}

	if err := legacy.createTables(); err != nil {
		t.Fatalf("migrate legacy schema: %v", err)
	}

	var updatedAt sql.NullString
	if err := rawDB.QueryRow(`SELECT updated_at FROM provider_models_cache WHERE instance_id = ?`, "legacy-provider").Scan(&updatedAt); err != nil {
		t.Fatalf("read migrated updated_at: %v", err)
	}
	if !updatedAt.Valid || !parseTime(updatedAt.String).Equal(time.Date(2026, 4, 25, 12, 34, 56, 0, time.UTC)) {
		t.Fatalf("expected updated_at to be backfilled from cached_at, got %#v", updatedAt)
	}

	cacheStore := &ProviderModelsCacheStore{db: legacy}
	if err := cacheStore.Save("legacy-provider", `{"models":["x"]}`); err != nil {
		t.Fatalf("save migrated cache row: %v", err)
	}

	if err := rawDB.QueryRow(`SELECT updated_at FROM provider_models_cache WHERE instance_id = ?`, "legacy-provider").Scan(&updatedAt); err != nil {
		t.Fatalf("read updated cache row: %v", err)
	}
	if !updatedAt.Valid || parseTime(updatedAt.String).Equal(time.Date(2026, 4, 25, 12, 34, 56, 0, time.UTC)) {
		t.Fatalf("expected save to refresh updated_at, got %#v", updatedAt)
	}
}

func TestParseTimeZeroValueForEmptyString(t *testing.T) {
	if got := parseTime(""); !got.Equal(time.Time{}) {
		t.Fatal("expected empty string to return zero time")
	}
}
