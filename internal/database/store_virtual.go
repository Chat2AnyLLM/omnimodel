package database

import (
	"database/sql"
	"fmt"
	"strings"
)

// ─── Virtual model operations ─────────────────────────────────────────────────

type VirtualModelStore struct {
	db *Database
}

func NewVirtualModelStore() *VirtualModelStore {
	return &VirtualModelStore{db: GetDatabase()}
}

func (s *VirtualModelStore) GetAll() ([]VirtualModelRecord, error) {
	rows, err := s.db.db.Query(`
		SELECT virtual_model_id, name, description, api_shape, lb_strategy, enabled, created_at, updated_at
		FROM virtual_models ORDER BY created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []VirtualModelRecord
	for rows.Next() {
		var r VirtualModelRecord
		var enabledInt int
		var createdAtStr, updatedAtStr string
		if err := rows.Scan(&r.VirtualModelID, &r.Name, &r.Description, &r.APIShape, &r.LbStrategy, &enabledInt, &createdAtStr, &updatedAtStr); err != nil {
			return nil, err
		}
		r.Enabled = enabledInt == 1
		r.CreatedAt = parseTime(createdAtStr)
		r.UpdatedAt = parseTime(updatedAtStr)
		records = append(records, r)
	}
	return records, nil
}

func (s *VirtualModelStore) Get(virtualModelID string) (*VirtualModelRecord, error) {
	var r VirtualModelRecord
	var enabledInt int
	var createdAtStr, updatedAtStr string
	err := s.db.db.QueryRow(`
		SELECT virtual_model_id, name, description, api_shape, lb_strategy, enabled, created_at, updated_at
		FROM virtual_models WHERE virtual_model_id = ? OR LOWER(virtual_model_id) = ?
		ORDER BY CASE WHEN virtual_model_id = ? THEN 0 ELSE 1 END, virtual_model_id ASC
		LIMIT 1
	`, virtualModelID, strings.ToLower(virtualModelID), virtualModelID).Scan(&r.VirtualModelID, &r.Name, &r.Description, &r.APIShape, &r.LbStrategy, &enabledInt, &createdAtStr, &updatedAtStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r.Enabled = enabledInt == 1
	r.CreatedAt = parseTime(createdAtStr)
	r.UpdatedAt = parseTime(updatedAtStr)
	return &r, nil
}

func (s *VirtualModelStore) Create(r *VirtualModelRecord) error {
	enabledInt := 0
	if r.Enabled {
		enabledInt = 1
	}
	_, err := s.db.db.Exec(`
		INSERT INTO virtual_models (virtual_model_id, name, description, api_shape, lb_strategy, enabled)
		VALUES (?, ?, ?, ?, ?, ?)
	`, r.VirtualModelID, r.Name, r.Description, r.APIShape, string(r.LbStrategy), enabledInt)
	return err
}

func (s *VirtualModelStore) Update(r *VirtualModelRecord) error {
	enabledInt := 0
	if r.Enabled {
		enabledInt = 1
	}
	_, err := s.db.db.Exec(`
		UPDATE virtual_models
		SET name = ?, description = ?, api_shape = ?, lb_strategy = ?, enabled = ?, updated_at = datetime('now')
		WHERE virtual_model_id = ?
	`, r.Name, r.Description, r.APIShape, string(r.LbStrategy), enabledInt, r.VirtualModelID)
	return err
}

func (s *VirtualModelStore) Delete(virtualModelID string) error {
	_, err := s.db.db.Exec("DELETE FROM virtual_models WHERE virtual_model_id = ?", virtualModelID)
	return err
}

func (s *VirtualModelStore) Rename(oldID string, newID string) error {
	tx, err := s.db.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Verify old ID exists
	var count int
	if err := tx.QueryRow("SELECT COUNT(*) FROM virtual_models WHERE virtual_model_id = ?", oldID).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("virtual model not found")
	}

	// Verify new ID does not exist
	if err := tx.QueryRow("SELECT COUNT(*) FROM virtual_models WHERE virtual_model_id = ?", newID).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("virtual model ID already exists")
	}

	if _, err := tx.Exec("UPDATE virtual_models SET virtual_model_id = ?, updated_at = datetime('now') WHERE virtual_model_id = ?", newID, oldID); err != nil {
		return err
	}
	if _, err := tx.Exec("UPDATE virtual_model_upstreams SET virtual_model_id = ? WHERE virtual_model_id = ?", newID, oldID); err != nil {
		return err
	}

	return tx.Commit()
}

// ─── Virtual model upstream operations ───────────────────────────────────────

type VirtualModelUpstreamStore struct {
	db *Database
}

func NewVirtualModelUpstreamStore() *VirtualModelUpstreamStore {
	return &VirtualModelUpstreamStore{db: GetDatabase()}
}

func (s *VirtualModelUpstreamStore) GetForVModel(virtualModelID string) ([]VirtualModelUpstreamRecord, error) {
	rows, err := s.db.db.Query(`
		SELECT id, virtual_model_id, provider_id, model_id, weight, priority, created_at, updated_at
		FROM virtual_model_upstreams
		WHERE virtual_model_id = ?
		ORDER BY priority ASC, id ASC
	`, virtualModelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []VirtualModelUpstreamRecord
	for rows.Next() {
		var r VirtualModelUpstreamRecord
		var createdAtStr, updatedAtStr string
		if err := rows.Scan(&r.ID, &r.VirtualModelID, &r.ProviderID, &r.ModelID, &r.Weight, &r.Priority, &createdAtStr, &updatedAtStr); err != nil {
			return nil, err
		}
		r.CreatedAt = parseTime(createdAtStr)
		r.UpdatedAt = parseTime(updatedAtStr)
		records = append(records, r)
	}
	return records, nil
}

// SetForVModel atomically replaces all upstreams for a virtual model.
func (s *VirtualModelUpstreamStore) SetForVModel(virtualModelID string, upstreams []VirtualModelUpstreamRecord) error {
	tx, err := s.db.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM virtual_model_upstreams WHERE virtual_model_id = ?", virtualModelID); err != nil {
		return err
	}
	for _, u := range upstreams {
		if _, err := tx.Exec(`
			INSERT INTO virtual_model_upstreams (virtual_model_id, provider_id, model_id, weight, priority, updated_at)
			VALUES (?, ?, ?, ?, ?, datetime('now'))
		`, virtualModelID, u.ProviderID, u.ModelID, u.Weight, u.Priority); err != nil {
			return err
		}
	}
	return tx.Commit()
}
