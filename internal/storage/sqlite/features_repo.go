package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"dezuxk/internal/domain/providers"
)

// ListFeatures returns features registered for a provider.
func (r *ProvidersRepository) ListFeatures(ctx context.Context, providerID string) ([]providers.ProviderFeature, error) {
	rows, err := r.db.sql.QueryContext(ctx, `
		SELECT id, provider_id, feature_key, name, zone_type, dag_node_type, execution_channel, inputs_schema_json, outputs_schema_json, params_schema_json, created_at
		FROM provider_features WHERE provider_id = ? ORDER BY name ASC
	`, providerID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list features for %s: %w", providerID, err)
	}
	defer rows.Close()

	var result []providers.ProviderFeature
	for rows.Next() {
		var f providers.ProviderFeature
		var inJSON, outJSON, paramJSON string
		if err := rows.Scan(&f.ID, &f.ProviderID, &f.FeatureKey, &f.Name, &f.ZoneType, &f.DAGNodeType, &f.ExecutionChannel, &inJSON, &outJSON, &paramJSON, &f.CreatedAt); err != nil {
			return nil, fmt.Errorf("sqlite: scan feature: %w", err)
		}
		f.InputsSchema = json.RawMessage(inJSON)
		f.OutputsSchema = json.RawMessage(outJSON)
		f.ParamsSchema = json.RawMessage(paramJSON)
		result = append(result, f)
	}
	return result, rows.Err()
}

// SaveFeature inserts or updates a provider feature.
func (r *ProvidersRepository) SaveFeature(ctx context.Context, f *providers.ProviderFeature) error {
	inJSON := string(f.InputsSchema)
	if inJSON == "" {
		inJSON = "[]"
	}
	outJSON := string(f.OutputsSchema)
	if outJSON == "" {
		outJSON = "[]"
	}
	paramJSON := string(f.ParamsSchema)
	if paramJSON == "" {
		paramJSON = "[]"
	}

	_, err := r.db.sql.ExecContext(ctx, `
		INSERT INTO provider_features (id, provider_id, feature_key, name, zone_type, dag_node_type, execution_channel, inputs_schema_json, outputs_schema_json, params_schema_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			provider_id = excluded.provider_id,
			feature_key = excluded.feature_key,
			name = excluded.name,
			zone_type = excluded.zone_type,
			dag_node_type = excluded.dag_node_type,
			execution_channel = excluded.execution_channel,
			inputs_schema_json = excluded.inputs_schema_json,
			outputs_schema_json = excluded.outputs_schema_json,
			params_schema_json = excluded.params_schema_json
	`, f.ID, f.ProviderID, f.FeatureKey, f.Name, string(f.ZoneType), f.DAGNodeType, string(f.ExecutionChannel), inJSON, outJSON, paramJSON)
	if err != nil {
		return fmt.Errorf("sqlite: save feature %s: %w", f.ID, err)
	}
	return nil
}

// GetEndpoint retrieves a dynamic endpoint by providerID and featureKey.
func (r *ProvidersRepository) GetEndpoint(ctx context.Context, providerID, featureKey string) (*providers.DynamicEndpoint, error) {
	row := r.db.sql.QueryRowContext(ctx, `
		SELECT id, provider_id, feature_key, http_method, url_template, headers_template_json, rpc_method_name, resolved_rpc_id, resolution_strategy, expires_at, updated_at
		FROM dynamic_endpoints WHERE provider_id = ? AND feature_key = ?
	`, providerID, featureKey)

	var ep providers.DynamicEndpoint
	var headersJSON string
	var expiresAt sql.NullTime

	err := row.Scan(&ep.ID, &ep.ProviderID, &ep.FeatureKey, &ep.HTTPMethod, &ep.URLTemplate, &headersJSON, &ep.RPCMethodName, &ep.ResolvedRPCID, &ep.ResolutionStrategy, &expiresAt, &ep.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("sqlite: get endpoint %s/%s: %w", providerID, featureKey, err)
	}

	if expiresAt.Valid {
		ep.ExpiresAt = &expiresAt.Time
	}
	_ = json.Unmarshal([]byte(headersJSON), &ep.HeadersTemplate)
	return &ep, nil
}

// SaveEndpoint inserts or updates a dynamic endpoint.
func (r *ProvidersRepository) SaveEndpoint(ctx context.Context, ep *providers.DynamicEndpoint) error {
	headersJSON, err := json.Marshal(ep.HeadersTemplate)
	if err != nil {
		return fmt.Errorf("sqlite: marshal headers template: %w", err)
	}

	_, err = r.db.sql.ExecContext(ctx, `
		INSERT INTO dynamic_endpoints (id, provider_id, feature_key, http_method, url_template, headers_template_json, rpc_method_name, resolved_rpc_id, resolution_strategy, expires_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(id) DO UPDATE SET
			provider_id = excluded.provider_id,
			feature_key = excluded.feature_key,
			http_method = excluded.http_method,
			url_template = excluded.url_template,
			headers_template_json = excluded.headers_template_json,
			rpc_method_name = excluded.rpc_method_name,
			resolved_rpc_id = excluded.resolved_rpc_id,
			resolution_strategy = excluded.resolution_strategy,
			expires_at = excluded.expires_at,
			updated_at = CURRENT_TIMESTAMP
	`, ep.ID, ep.ProviderID, ep.FeatureKey, ep.HTTPMethod, ep.URLTemplate, string(headersJSON), ep.RPCMethodName, ep.ResolvedRPCID, ep.ResolutionStrategy, ep.ExpiresAt)
	if err != nil {
		return fmt.Errorf("sqlite: save endpoint %s: %w", ep.ID, err)
	}
	return nil
}

// ListCookieRules returns cookie partition rules for a provider.
func (r *ProvidersRepository) ListCookieRules(ctx context.Context, providerID string) ([]providers.CookiePartitionRule, error) {
	rows, err := r.db.sql.QueryContext(ctx, `
		SELECT id, provider_id, zone_type, domain_pattern, name_pattern, is_required
		FROM cookie_partition_rules WHERE provider_id = ?
	`, providerID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list cookie rules for %s: %w", providerID, err)
	}
	defer rows.Close()

	var result []providers.CookiePartitionRule
	for rows.Next() {
		var rule providers.CookiePartitionRule
		var reqInt int
		if err := rows.Scan(&rule.ID, &rule.ProviderID, &rule.ZoneType, &rule.DomainPattern, &rule.NamePattern, &reqInt); err != nil {
			return nil, fmt.Errorf("sqlite: scan cookie rule: %w", err)
		}
		rule.IsRequired = reqInt == 1
		result = append(result, rule)
	}
	return result, rows.Err()
}

// SaveCookieRule inserts or updates a cookie partition rule.
func (r *ProvidersRepository) SaveCookieRule(ctx context.Context, rule *providers.CookiePartitionRule) error {
	reqInt := 0
	if rule.IsRequired {
		reqInt = 1
	}

	_, err := r.db.sql.ExecContext(ctx, `
		INSERT INTO cookie_partition_rules (id, provider_id, zone_type, domain_pattern, name_pattern, is_required)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			provider_id = excluded.provider_id,
			zone_type = excluded.zone_type,
			domain_pattern = excluded.domain_pattern,
			name_pattern = excluded.name_pattern,
			is_required = excluded.is_required
	`, rule.ID, rule.ProviderID, string(rule.ZoneType), rule.DomainPattern, rule.NamePattern, reqInt)
	if err != nil {
		return fmt.Errorf("sqlite: save cookie rule %s: %w", rule.ID, err)
	}
	return nil
}
