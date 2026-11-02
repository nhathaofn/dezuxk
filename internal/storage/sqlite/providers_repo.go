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

// GetProvider retrieves a provider manifest by ID.
func (r *ProvidersRepository) GetProvider(ctx context.Context, id string) (*providers.Provider, error) {
	row := r.db.sql.QueryRowContext(ctx, `
		SELECT id, name, base_url, auth_domain, allowed_domains_json, probe_rules_json, security_rules_json, status, created_at, updated_at
		FROM dynamic_providers WHERE id = ?
	`, id)

	var p providers.Provider
	var allowedJSON, probeJSON, secJSON string
	var createdAt, updatedAt time.Time

	err := row.Scan(&p.ID, &p.Name, &p.BaseURL, &p.AuthDomain, &allowedJSON, &probeJSON, &secJSON, &p.Status, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("sqlite: get provider %s: %w", id, err)
	}

	p.CreatedAt = createdAt
	p.UpdatedAt = updatedAt
	if err := json.Unmarshal([]byte(allowedJSON), &p.AllowedDomains); err != nil {
		p.AllowedDomains = []string{}
	}
	if err := json.Unmarshal([]byte(probeJSON), &p.ProbeRules); err != nil {
		p.ProbeRules = providers.ProbeRules{}
	}
	if err := json.Unmarshal([]byte(secJSON), &p.SecurityRules); err != nil {
		p.SecurityRules = map[string]any{}
	}

	return &p, nil
}

// ListProviders returns all registered dynamic providers.
func (r *ProvidersRepository) ListProviders(ctx context.Context) ([]providers.Provider, error) {
	rows, err := r.db.sql.QueryContext(ctx, `
		SELECT id, name, base_url, auth_domain, allowed_domains_json, probe_rules_json, security_rules_json, status, created_at, updated_at
		FROM dynamic_providers ORDER BY name ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list providers: %w", err)
	}
	defer rows.Close()

	var result []providers.Provider
	for rows.Next() {
		var p providers.Provider
		var allowedJSON, probeJSON, secJSON string
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&p.ID, &p.Name, &p.BaseURL, &p.AuthDomain, &allowedJSON, &probeJSON, &secJSON, &p.Status, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("sqlite: scan provider: %w", err)
		}
		p.CreatedAt = createdAt
		p.UpdatedAt = updatedAt
		_ = json.Unmarshal([]byte(allowedJSON), &p.AllowedDomains)
		_ = json.Unmarshal([]byte(probeJSON), &p.ProbeRules)
		_ = json.Unmarshal([]byte(secJSON), &p.SecurityRules)
		result = append(result, p)
	}
	return result, rows.Err()
}

// SaveProvider inserts or updates a dynamic provider manifest.
func (r *ProvidersRepository) SaveProvider(ctx context.Context, p *providers.Provider) error {
	allowedJSON, err := json.Marshal(p.AllowedDomains)
	if err != nil {
		return fmt.Errorf("sqlite: marshal allowed domains: %w", err)
	}
	probeJSON, err := json.Marshal(p.ProbeRules)
	if err != nil {
		return fmt.Errorf("sqlite: marshal probe rules: %w", err)
	}
	secJSON, err := json.Marshal(p.SecurityRules)
	if err != nil {
		return fmt.Errorf("sqlite: marshal security rules: %w", err)
	}

	_, err = r.db.sql.ExecContext(ctx, `
		INSERT INTO dynamic_providers (id, name, base_url, auth_domain, allowed_domains_json, probe_rules_json, security_rules_json, status, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			base_url = excluded.base_url,
			auth_domain = excluded.auth_domain,
			allowed_domains_json = excluded.allowed_domains_json,
			probe_rules_json = excluded.probe_rules_json,
			security_rules_json = excluded.security_rules_json,
			status = excluded.status,
			updated_at = CURRENT_TIMESTAMP
	`, p.ID, p.Name, p.BaseURL, p.AuthDomain, string(allowedJSON), string(probeJSON), string(secJSON), string(p.Status))
	if err != nil {
		return fmt.Errorf("sqlite: save provider %s: %w", p.ID, err)
	}
	return nil
}

// DeleteProvider deletes a provider (cascading to accounts, secrets, endpoints).
func (r *ProvidersRepository) DeleteProvider(ctx context.Context, id string) error {
	_, err := r.db.sql.ExecContext(ctx, "DELETE FROM dynamic_providers WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("sqlite: delete provider %s: %w", id, err)
	}
	return nil
}
