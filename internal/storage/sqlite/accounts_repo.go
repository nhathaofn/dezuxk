package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"dezuxk/internal/domain/providers"
)

// GetAccount retrieves an account by ID.
func (r *ProvidersRepository) GetAccount(ctx context.Context, id string) (*providers.Account, error) {
	row := r.db.sql.QueryRowContext(ctx, `
		SELECT id, provider_id, label, identity, status, profile_dir, proxy_id, paused_until, usage_json, metadata_json, last_checked_at, created_at, updated_at
		FROM accounts WHERE id = ?
	`, id)

	var acc providers.Account
	var usageJSON, metaJSON string
	var pausedUntil, lastChecked sql.NullTime
	var createdAt, updatedAt time.Time

	err := row.Scan(&acc.ID, &acc.ProviderID, &acc.Label, &acc.Identity, &acc.Status, &acc.ProfileDir, &acc.ProxyID, &pausedUntil, &usageJSON, &metaJSON, &lastChecked, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("sqlite: get account %s: %w", id, err)
	}

	acc.CreatedAt = createdAt
	acc.UpdatedAt = updatedAt
	if pausedUntil.Valid {
		acc.PausedUntil = &pausedUntil.Time
	}
	if lastChecked.Valid {
		acc.LastCheckedAt = &lastChecked.Time
	}
	_ = json.Unmarshal([]byte(usageJSON), &acc.Usage)
	_ = json.Unmarshal([]byte(metaJSON), &acc.Metadata)

	return &acc, nil
}

// ListAccounts lists all accounts for a specific provider (or all providers if providerID is empty).
func (r *ProvidersRepository) ListAccounts(ctx context.Context, providerID string) ([]providers.Account, error) {
	query := `
		SELECT id, provider_id, label, identity, status, profile_dir, proxy_id, paused_until, usage_json, metadata_json, last_checked_at, created_at, updated_at
		FROM accounts
	`
	var args []any
	if providerID != "" {
		query += " WHERE provider_id = ?"
		args = append(args, providerID)
	}
	query += " ORDER BY label ASC"

	rows, err := r.db.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list accounts: %w", err)
	}
	defer rows.Close()

	var result []providers.Account
	for rows.Next() {
		var acc providers.Account
		var usageJSON, metaJSON string
		var pausedUntil, lastChecked sql.NullTime
		var createdAt, updatedAt time.Time

		if err := rows.Scan(&acc.ID, &acc.ProviderID, &acc.Label, &acc.Identity, &acc.Status, &acc.ProfileDir, &acc.ProxyID, &pausedUntil, &usageJSON, &metaJSON, &lastChecked, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("sqlite: scan account: %w", err)
		}
		acc.CreatedAt = createdAt
		acc.UpdatedAt = updatedAt
		if pausedUntil.Valid {
			acc.PausedUntil = &pausedUntil.Time
		}
		if lastChecked.Valid {
			acc.LastCheckedAt = &lastChecked.Time
		}
		_ = json.Unmarshal([]byte(usageJSON), &acc.Usage)
		_ = json.Unmarshal([]byte(metaJSON), &acc.Metadata)
		result = append(result, acc)
	}
	return result, rows.Err()
}

// SaveAccount inserts or updates an account.
func (r *ProvidersRepository) SaveAccount(ctx context.Context, acc *providers.Account) error {
	usageJSON, err := json.Marshal(acc.Usage)
	if err != nil {
		return fmt.Errorf("sqlite: marshal usage: %w", err)
	}
	metaJSON, err := json.Marshal(acc.Metadata)
	if err != nil {
		return fmt.Errorf("sqlite: marshal metadata: %w", err)
	}

	var pausedUTC *time.Time
	if acc.PausedUntil != nil {
		t := acc.PausedUntil.UTC()
		pausedUTC = &t
	}

	var lastCheckedUTC *time.Time
	if acc.LastCheckedAt != nil {
		t := acc.LastCheckedAt.UTC()
		lastCheckedUTC = &t
	}

	_, err = r.db.sql.ExecContext(ctx, `
		INSERT INTO accounts (id, provider_id, label, identity, status, profile_dir, proxy_id, paused_until, usage_json, metadata_json, last_checked_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(id) DO UPDATE SET
			provider_id = excluded.provider_id,
			label = excluded.label,
			identity = excluded.identity,
			status = excluded.status,
			profile_dir = excluded.profile_dir,
			proxy_id = excluded.proxy_id,
			paused_until = excluded.paused_until,
			usage_json = excluded.usage_json,
			metadata_json = excluded.metadata_json,
			last_checked_at = excluded.last_checked_at,
			updated_at = CURRENT_TIMESTAMP
	`, acc.ID, acc.ProviderID, acc.Label, acc.Identity, string(acc.Status), acc.ProfileDir, acc.ProxyID, pausedUTC, string(usageJSON), string(metaJSON), lastCheckedUTC)
	if err != nil {
		return fmt.Errorf("sqlite: save account %s: %w", acc.ID, err)
	}
	return nil
}

// DeleteAccount deletes an account by ID (cascades to secrets).
func (r *ProvidersRepository) DeleteAccount(ctx context.Context, id string) error {
	_, err := r.db.sql.ExecContext(ctx, "DELETE FROM accounts WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("sqlite: delete account %s: %w", id, err)
	}
	return nil
}

// UpdateAccountStatus updates only the status and paused_until of an account.
func (r *ProvidersRepository) UpdateAccountStatus(ctx context.Context, id string, status providers.AccountStatus, pausedUntil *time.Time) error {
	var pausedUTC *time.Time
	if pausedUntil != nil {
		t := pausedUntil.UTC()
		pausedUTC = &t
	}

	_, err := r.db.sql.ExecContext(ctx, `
		UPDATE accounts SET status = ?, paused_until = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?
	`, string(status), pausedUTC, id)
	if err != nil {
		return fmt.Errorf("sqlite: update account status %s: %w", id, err)
	}
	return nil
}

// CountReadyAccounts returns the count of accounts in 'ready' status that are not currently paused.
func (r *ProvidersRepository) CountReadyAccounts(ctx context.Context) (int, error) {
	var count int
	err := r.db.sql.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM accounts
		WHERE status = 'ready' AND (paused_until IS NULL OR paused_until <= ?)
	`, time.Now().UTC()).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("sqlite: count ready accounts: %w", err)
	}
	return count, nil
}

// CountTotalAccounts returns the total number of registered accounts.
func (r *ProvidersRepository) CountTotalAccounts(ctx context.Context) (int, error) {
	var count int
	err := r.db.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM accounts`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("sqlite: count total accounts: %w", err)
	}
	return count, nil
}

// ListReadyAccounts returns ready accounts for a provider (or all providers if providerID is empty),
// ordered by last_checked_at ASC to enable round-robin / fair usage rotation.
func (r *ProvidersRepository) ListReadyAccounts(ctx context.Context, providerID string) ([]providers.Account, error) {
	query := `
		SELECT id, provider_id, label, identity, status, profile_dir, proxy_id, paused_until, usage_json, metadata_json, last_checked_at, created_at, updated_at
		FROM accounts
		WHERE status = 'ready' AND (paused_until IS NULL OR paused_until <= ?)
	`
	args := []any{time.Now().UTC()}
	if providerID != "" {
		query += " AND provider_id = ?"
		args = append(args, providerID)
	}
	query += " ORDER BY last_checked_at ASC, updated_at ASC"

	rows, err := r.db.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list ready accounts: %w", err)
	}
	defer rows.Close()

	var result []providers.Account
	for rows.Next() {
		var acc providers.Account
		var usageJSON, metaJSON string
		var pausedUntil, lastChecked sql.NullTime
		var createdAt, updatedAt time.Time

		if err := rows.Scan(&acc.ID, &acc.ProviderID, &acc.Label, &acc.Identity, &acc.Status, &acc.ProfileDir, &acc.ProxyID, &pausedUntil, &usageJSON, &metaJSON, &lastChecked, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("sqlite: scan ready account: %w", err)
		}
		acc.CreatedAt = createdAt
		acc.UpdatedAt = updatedAt
		if pausedUntil.Valid {
			acc.PausedUntil = &pausedUntil.Time
		}
		if lastChecked.Valid {
			acc.LastCheckedAt = &lastChecked.Time
		}
		_ = json.Unmarshal([]byte(usageJSON), &acc.Usage)
		_ = json.Unmarshal([]byte(metaJSON), &acc.Metadata)
		result = append(result, acc)
	}
	return result, rows.Err()
}
