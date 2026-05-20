package database

import (
	"database/sql"
	"time"
)

// ─── Provider models cache operations ─────────────────────────────────────────

// DefaultCacheTTL is the default time-to-live for cached model lists (24 hours).
const DefaultCacheTTL = 24 * time.Hour

type ProviderModelsCacheStore struct {
	db *Database
}

func NewProviderModelsCacheStore() *ProviderModelsCacheStore {
	return &ProviderModelsCacheStore{db: GetDatabase()}
}

// Get returns the cached model list if it exists, matches the provider type, and is still valid.
// Returns nil, nil if no cache entry exists or it has expired.
func (cs *ProviderModelsCacheStore) Get(instanceID, providerID string, ttl time.Duration) (*ProviderModelsCacheRecord, error) {
	var record ProviderModelsCacheRecord
	var cachedAtStr string
	err := cs.db.db.QueryRow(`
		SELECT instance_id, provider_id, models_data, cached_at
		FROM provider_models_cache WHERE instance_id = ? AND provider_id = ?
	`, instanceID, providerID).Scan(&record.InstanceID, &record.ProviderID, &record.ModelsData, &cachedAtStr)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	record.CachedAt = parseTime(cachedAtStr)

	if time.Since(record.CachedAt) > ttl {
		return nil, nil
	}

	return &record, nil
}

// Save stores the model list in the cache.
func (cs *ProviderModelsCacheStore) Save(instanceID, providerID, modelsData string) error {
	_, err := cs.db.db.Exec(`
		INSERT INTO provider_models_cache (instance_id, provider_id, models_data, cached_at, updated_at)
		VALUES (?, ?, ?, datetime('now'), datetime('now'))
		ON CONFLICT(instance_id) DO UPDATE SET
			provider_id = excluded.provider_id,
			models_data = excluded.models_data,
			cached_at = datetime('now'),
			updated_at = datetime('now')
	`, instanceID, providerID, modelsData)
	return err
}

// Delete removes the cache entry for a provider instance.
func (cs *ProviderModelsCacheStore) Delete(instanceID string) error {
	_, err := cs.db.db.Exec("DELETE FROM provider_models_cache WHERE instance_id = ?", instanceID)
	return err
}
