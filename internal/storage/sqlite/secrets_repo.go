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

// GetAccountSecrets retrieves and transparently decrypts cookies and tokens via the injected Cipher.
func (r *ProvidersRepository) GetAccountSecrets(ctx context.Context, accountID string) (*providers.AccountSecrets, error) {
	row := r.db.sql.QueryRowContext(ctx, `
		SELECT encrypted_cookies_blob, encrypted_tokens_blob, cookie_hash, updated_at
		FROM account_secrets WHERE account_id = ?
	`, accountID)

	var encryptedCookies, encryptedTokens []byte
	var cookieHash string
	var updatedAt time.Time

	err := row.Scan(&encryptedCookies, &encryptedTokens, &cookieHash, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("sqlite: get account secrets for %s: %w", accountID, err)
	}

	sec := &providers.AccountSecrets{
		AccountID:  accountID,
		CookieHash: cookieHash,
		UpdatedAt:  updatedAt,
		Cookies:    []providers.Cookie{},
		Tokens:     map[string]string{},
	}

	if len(encryptedCookies) > 0 {
		plainCookies, err := r.cipher.Decrypt(encryptedCookies)
		if err != nil {
			return nil, fmt.Errorf("sqlite: decrypt cookies for %s: %w", accountID, err)
		}
		if err := json.Unmarshal(plainCookies, &sec.Cookies); err != nil {
			return nil, fmt.Errorf("sqlite: unmarshal cookies for %s: %w", accountID, err)
		}
	}

	if len(encryptedTokens) > 0 {
		plainTokens, err := r.cipher.Decrypt(encryptedTokens)
		if err != nil {
			return nil, fmt.Errorf("sqlite: decrypt tokens for %s: %w", accountID, err)
		}
		if err := json.Unmarshal(plainTokens, &sec.Tokens); err != nil {
			return nil, fmt.Errorf("sqlite: unmarshal tokens for %s: %w", accountID, err)
		}
	}

	return sec, nil
}

// SaveAccountSecrets transparently encrypts cookies and tokens via the injected Cipher before storing.
func (r *ProvidersRepository) SaveAccountSecrets(ctx context.Context, sec *providers.AccountSecrets) error {
	cookiesJSON, err := json.Marshal(sec.Cookies)
	if err != nil {
		return fmt.Errorf("sqlite: marshal cookies: %w", err)
	}
	tokensJSON, err := json.Marshal(sec.Tokens)
	if err != nil {
		return fmt.Errorf("sqlite: marshal tokens: %w", err)
	}

	encryptedCookies, err := r.cipher.Encrypt(cookiesJSON)
	if err != nil {
		return fmt.Errorf("sqlite: encrypt cookies: %w", err)
	}

	encryptedTokens, err := r.cipher.Encrypt(tokensJSON)
	if err != nil {
		return fmt.Errorf("sqlite: encrypt tokens: %w", err)
	}

	_, err = r.db.sql.ExecContext(ctx, `
		INSERT INTO account_secrets (account_id, encrypted_cookies_blob, encrypted_tokens_blob, cookie_hash, updated_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(account_id) DO UPDATE SET
			encrypted_cookies_blob = excluded.encrypted_cookies_blob,
			encrypted_tokens_blob = excluded.encrypted_tokens_blob,
			cookie_hash = excluded.cookie_hash,
			updated_at = CURRENT_TIMESTAMP
	`, sec.AccountID, encryptedCookies, encryptedTokens, sec.CookieHash)
	if err != nil {
		return fmt.Errorf("sqlite: save account secrets for %s: %w", sec.AccountID, err)
	}
	return nil
}

// DeleteAccountSecrets deletes account secrets for an account.
func (r *ProvidersRepository) DeleteAccountSecrets(ctx context.Context, accountID string) error {
	_, err := r.db.sql.ExecContext(ctx, "DELETE FROM account_secrets WHERE account_id = ?", accountID)
	if err != nil {
		return fmt.Errorf("sqlite: delete account secrets for %s: %w", accountID, err)
	}
	return nil
}
