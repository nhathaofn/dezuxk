package db

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"dezuxk/internal/crypto"
	"dezuxk/internal/models"
)

func (d *DB) encryptField(val string) (string, error) {
	if val == "" {
		return val, nil
	}
	if len(d.cipherKey) != 32 {
		return "", errors.New("sensitive-data encryption key is unavailable")
	}
	return crypto.Encrypt(val, d.cipherKey)
}

func (d *DB) decryptField(val string) (string, error) {
	if val == "" {
		return "", nil
	}
	if len(d.cipherKey) != 32 {
		return "", errors.New("sensitive-data encryption key is unavailable")
	}
	return crypto.Decrypt(val, d.cipherKey)
}

// migrateField upgrades plaintext or legacy-key ciphertext to the current
// key. Current ciphertext is retained to avoid needless nonce churn on every
// application start.
func (d *DB) migrateField(val string) (string, error) {
	if val == "" {
		return "", nil
	}
	if !strings.HasPrefix(val, crypto.EncryptedPrefix) {
		return d.encryptField(val)
	}

	if _, err := d.decryptField(val); err == nil {
		return val, nil
	}
	if len(d.legacyCipherKey) == 32 && !bytes.Equal(d.legacyCipherKey, d.cipherKey) {
		plain, err := crypto.Decrypt(val, d.legacyCipherKey)
		if err != nil {
			return "", fmt.Errorf("failed to decrypt legacy sensitive field: %w", err)
		}
		return d.encryptField(plain)
	}
	return "", errors.New("failed to decrypt sensitive field with the current key")
}

func (d *DB) decryptAccountSecrets(acc *models.GoogleAccount) error {
	var err error
	if acc.Cookies, err = d.decryptField(acc.Cookies); err != nil {
		return fmt.Errorf("failed to decrypt cookies for account %s: %w", acc.ID, err)
	}
	if acc.SNlM0eToken, err = d.decryptField(acc.SNlM0eToken); err != nil {
		return fmt.Errorf("failed to decrypt session token for account %s: %w", acc.ID, err)
	}
	if acc.Proxy, err = d.decryptField(acc.Proxy); err != nil {
		return fmt.Errorf("failed to decrypt proxy for account %s: %w", acc.ID, err)
	}
	// Password and recovery_email are legacy columns. They are intentionally not
	// loaded into the model and are cleared by migrateSensitiveFields.
	return nil
}

// migrateSensitiveFields upgrades legacy plaintext session data in place. The
// migration is idempotent and clears the old Google password/recovery columns.
func (d *DB) migrateSensitiveFields() error {
	if len(d.cipherKey) != 32 {
		return errors.New("sensitive-data encryption key is unavailable")
	}

	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	rows, err := tx.Query(`SELECT id, cookies, snlm0e_token, proxy FROM google_accounts`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type rowData struct {
		id, cookies, token, proxy string
	}
	var records []rowData
	for rows.Next() {
		var record rowData
		if err := rows.Scan(&record.id, &record.cookies, &record.token, &record.proxy); err != nil {
			return err
		}
		record.cookies, err = d.migrateField(record.cookies)
		if err != nil {
			return err
		}
		record.token, err = d.migrateField(record.token)
		if err != nil {
			return err
		}
		record.proxy, err = d.migrateField(record.proxy)
		if err != nil {
			return err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}

	for _, record := range records {
		if _, err := tx.Exec(`UPDATE google_accounts
			SET cookies = ?, snlm0e_token = ?, proxy = ?, password = '', recovery_email = ''
			WHERE id = ?`, record.cookies, record.token, record.proxy, record.id); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(`UPDATE google_accounts SET password = '', recovery_email = ''`); err != nil {
		return err
	}
	return tx.Commit()
}

// CreateGoogleAccount inserts a new Google account into the database.
func (d *DB) CreateGoogleAccount(acc *models.GoogleAccount) error {
	if acc == nil || acc.ID == "" {
		return errors.New("invalid account data")
	}
	acc.Email = strings.ToLower(strings.TrimSpace(acc.Email))
	if acc.Email == "" {
		return errors.New("Google account email is required")
	}
	acc.Tier = normalizeTierValue(acc.Tier)
	if acc.Credits < 0 {
		acc.Credits = 0
	}

	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	if acc.CreatedAt == "" {
		acc.CreatedAt = now
	}
	acc.UpdatedAt = now

	query := `INSERT INTO google_accounts (
		id, email, name, avatar_url, profile_dir, cookies, snlm0e_token,
		services, status, tier, credits, proxy, image_enabled, video_enabled, user_agent,
		last_error, last_refresh_at, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	imgEnabled := 1
	if !acc.ImageEnabled && acc.CreatedAt != acc.UpdatedAt {
		imgEnabled = 0
	}
	vidEnabled := 1
	if !acc.VideoEnabled && acc.CreatedAt != acc.UpdatedAt {
		vidEnabled = 0
	}

	encCookies, err := d.encryptField(acc.Cookies)
	if err != nil {
		return fmt.Errorf("failed to encrypt cookies: %w", err)
	}
	encSnlm0e, err := d.encryptField(acc.SNlM0eToken)
	if err != nil {
		return fmt.Errorf("failed to encrypt session token: %w", err)
	}
	encProxy, err := d.encryptField(acc.Proxy)
	if err != nil {
		return fmt.Errorf("failed to encrypt proxy: %w", err)
	}

	_, err = d.conn.Exec(
		query,
		acc.ID,
		acc.Email,
		acc.Name,
		acc.AvatarURL,
		acc.ProfileDir,
		encCookies,
		encSnlm0e,
		acc.Services,
		acc.Status,
		acc.Tier,
		acc.Credits,
		encProxy,
		imgEnabled,
		vidEnabled,
		acc.UserAgent,
		acc.LastError,
		acc.LastRefreshAt,
		acc.CreatedAt,
		acc.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert google account: %w", err)
	}

	return nil
}

// UpsertGoogleAccount inserts or updates a Google account record.
func (d *DB) UpsertGoogleAccount(acc *models.GoogleAccount) error {
	if acc == nil || acc.ID == "" {
		return errors.New("invalid account data")
	}
	acc.Email = strings.ToLower(strings.TrimSpace(acc.Email))
	if acc.Email == "" {
		return errors.New("Google account email is required")
	}
	if strings.TrimSpace(acc.Tier) != "" {
		acc.Tier = normalizeTierValue(acc.Tier)
	}
	if acc.Credits < 0 {
		acc.Credits = 0
	}

	existing, err := d.GetGoogleAccountByEmail(acc.Email)
	if err == nil && existing != nil {
		// Update existing record
		now := time.Now().UTC().Format("2006-01-02 15:04:05")
		acc.ID = existing.ID
		if acc.ProfileDir == "" {
			acc.ProfileDir = existing.ProfileDir
		}

		encCookies, err := d.encryptField(acc.Cookies)
		if err != nil {
			return fmt.Errorf("failed to encrypt cookies: %w", err)
		}
		encSnlm0e, err := d.encryptField(acc.SNlM0eToken)
		if err != nil {
			return fmt.Errorf("failed to encrypt session token: %w", err)
		}
		encProxy, err := d.encryptField(acc.Proxy)
		if err != nil {
			return fmt.Errorf("failed to encrypt proxy: %w", err)
		}

		query := `UPDATE google_accounts SET
			cookies = ?,
			snlm0e_token = CASE WHEN ? != '' THEN ? ELSE snlm0e_token END,
			tier = CASE WHEN ? != '' THEN ? ELSE tier END,
			credits = CASE WHEN ? >= 0 THEN ? ELSE credits END,
			proxy = CASE WHEN ? != '' THEN ? ELSE proxy END,
			status = ?,
			services = ?,
			profile_dir = ?,
			last_refresh_at = CASE WHEN ? != '' THEN ? ELSE last_refresh_at END,
			updated_at = ?
		WHERE id = ?`

		_, err = d.conn.Exec(
			query,
			encCookies,
			encSnlm0e, encSnlm0e,
			acc.Tier, acc.Tier,
			acc.Credits, acc.Credits,
			encProxy, encProxy,
			acc.Status,
			acc.Services,
			acc.ProfileDir,
			acc.LastRefreshAt,
			acc.LastRefreshAt,
			now,
			existing.ID,
		)
		return err
	}

	return d.CreateGoogleAccount(acc)
}

// GetGoogleAccountByID retrieves a single Google account by its primary key ID.
func (d *DB) GetGoogleAccountByID(id string) (*models.GoogleAccount, error) {
	query := `SELECT id, email, name, avatar_url, profile_dir, cookies, snlm0e_token,
		services, status, COALESCE(tier, 'FREE'), COALESCE(credits, 0), COALESCE(proxy, ''),
		COALESCE(image_enabled, 1), COALESCE(video_enabled, 1), COALESCE(user_agent, ''),
		COALESCE(last_error, ''), COALESCE(last_refresh_at, ''),
		created_at, updated_at
	FROM google_accounts WHERE id = ?`

	row := d.conn.QueryRow(query, id)

	var acc models.GoogleAccount
	var imgEnabled, vidEnabled int
	err := row.Scan(
		&acc.ID,
		&acc.Email,
		&acc.Name,
		&acc.AvatarURL,
		&acc.ProfileDir,
		&acc.Cookies,
		&acc.SNlM0eToken,
		&acc.Services,
		&acc.Status,
		&acc.Tier,
		&acc.Credits,
		&acc.Proxy,
		&imgEnabled,
		&vidEnabled,
		&acc.UserAgent,
		&acc.LastError,
		&acc.LastRefreshAt,
		&acc.CreatedAt,
		&acc.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("google account not found")
		}
		return nil, err
	}

	acc.ImageEnabled = imgEnabled == 1
	acc.VideoEnabled = vidEnabled == 1

	// Transparently decrypt sensitive fields
	if err := d.decryptAccountSecrets(&acc); err != nil {
		return nil, err
	}

	return &acc, nil
}

// GetGoogleAccountByEmail retrieves an account by email address.
func (d *DB) GetGoogleAccountByEmail(email string) (*models.GoogleAccount, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	query := `SELECT id, email, name, avatar_url, profile_dir, cookies, snlm0e_token,
		services, status, COALESCE(tier, 'FREE'), COALESCE(credits, 0), COALESCE(proxy, ''),
		COALESCE(image_enabled, 1), COALESCE(video_enabled, 1), COALESCE(user_agent, ''),
		COALESCE(last_error, ''), COALESCE(last_refresh_at, ''),
		created_at, updated_at
	FROM google_accounts WHERE email = ? LIMIT 1`

	row := d.conn.QueryRow(query, email)

	var acc models.GoogleAccount
	var imgEnabled, vidEnabled int
	err := row.Scan(
		&acc.ID,
		&acc.Email,
		&acc.Name,
		&acc.AvatarURL,
		&acc.ProfileDir,
		&acc.Cookies,
		&acc.SNlM0eToken,
		&acc.Services,
		&acc.Status,
		&acc.Tier,
		&acc.Credits,
		&acc.Proxy,
		&imgEnabled,
		&vidEnabled,
		&acc.UserAgent,
		&acc.LastError,
		&acc.LastRefreshAt,
		&acc.CreatedAt,
		&acc.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("google account not found")
		}
		return nil, err
	}

	acc.ImageEnabled = imgEnabled == 1
	acc.VideoEnabled = vidEnabled == 1

	// Transparently decrypt sensitive fields
	if err := d.decryptAccountSecrets(&acc); err != nil {
		return nil, err
	}

	return &acc, nil
}

// ListGoogleAccounts returns all Google accounts ordered by creation time descending.
func (d *DB) ListGoogleAccounts() ([]*models.GoogleAccount, error) {
	query := `SELECT id, email, name, avatar_url, profile_dir, cookies, snlm0e_token,
		services, status, COALESCE(tier, 'FREE'), COALESCE(credits, 0), COALESCE(proxy, ''),
		COALESCE(image_enabled, 1), COALESCE(video_enabled, 1), COALESCE(user_agent, ''),
		COALESCE(last_error, ''), COALESCE(last_refresh_at, ''),
		created_at, updated_at
	FROM google_accounts ORDER BY created_at DESC`

	rows, err := d.conn.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query google accounts: %w", err)
	}
	defer rows.Close()

	var accounts []*models.GoogleAccount
	for rows.Next() {
		var acc models.GoogleAccount
		var imgEnabled, vidEnabled int
		err := rows.Scan(
			&acc.ID,
			&acc.Email,
			&acc.Name,
			&acc.AvatarURL,
			&acc.ProfileDir,
			&acc.Cookies,
			&acc.SNlM0eToken,
			&acc.Services,
			&acc.Status,
			&acc.Tier,
			&acc.Credits,
			&acc.Proxy,
			&imgEnabled,
			&vidEnabled,
			&acc.UserAgent,
			&acc.LastError,
			&acc.LastRefreshAt,
			&acc.CreatedAt,
			&acc.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan google account: %w", err)
		}
		acc.ImageEnabled = imgEnabled == 1
		acc.VideoEnabled = vidEnabled == 1

		// Transparently decrypt sensitive fields
		if err := d.decryptAccountSecrets(&acc); err != nil {
			return nil, err
		}

		accounts = append(accounts, &acc)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return accounts, nil
}

// UpdateGoogleAccountCookies updates cookies, CSRF token, and touches last_refresh_at.
func (d *DB) UpdateGoogleAccountCookies(id, cookies, snlm0e string) error {
	return d.UpdateGoogleAccountSessionData(id, cookies, snlm0e, -1, "")
}

// UpdateGoogleAccountSessionData updates cookies, CSRF token, credits, tier, and touches last_refresh_at.
func (d *DB) UpdateGoogleAccountSessionData(id, cookies, snlm0e string, credits int, tier string) error {
	now := time.Now().UTC().Format("2006-01-02 15:04:05")

	encCookies, err := d.encryptField(cookies)
	if err != nil {
		return fmt.Errorf("failed to encrypt cookies: %w", err)
	}
	encSnlm0e, err := d.encryptField(snlm0e)
	if err != nil {
		return fmt.Errorf("failed to encrypt session token: %w", err)
	}

	query := `UPDATE google_accounts SET
		cookies = ?,
		snlm0e_token = CASE WHEN ? != '' THEN ? ELSE snlm0e_token END,
		credits = CASE WHEN ? >= 0 THEN ? ELSE credits END,
		tier = CASE WHEN ? != '' THEN ? ELSE tier END,
		status = 'ACTIVE',
		last_error = '',
		last_refresh_at = ?,
		updated_at = ?
	WHERE id = ?`

	res, err := d.conn.Exec(query, encCookies, encSnlm0e, encSnlm0e, credits, credits, tier, tier, now, now, id)
	if err != nil {
		return fmt.Errorf("failed to update session data for account %s: %w", id, err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return errors.New("google account not found")
	}

	return nil
}

// UpdateGoogleAccountCredits updates the credit count of an account directly.
func (d *DB) UpdateGoogleAccountCredits(id string, credits int) error {
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	query := `UPDATE google_accounts SET credits = ?, updated_at = ? WHERE id = ?`
	res, err := d.conn.Exec(query, credits, now, id)
	if err != nil {
		return fmt.Errorf("failed to update credits for account %s: %w", id, err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return errors.New("google account not found")
	}
	return nil
}

// UpdateGoogleAccountStatus updates the status and last error of an account.
func (d *DB) UpdateGoogleAccountStatus(id, status, lastError string) error {
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	query := `UPDATE google_accounts SET status = ?, last_error = ?, updated_at = ? WHERE id = ?`

	res, err := d.conn.Exec(query, status, lastError, now, id)
	if err != nil {
		return fmt.Errorf("failed to update status for account %s: %w", id, err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return errors.New("google account not found")
	}

	return nil
}

// UpdateGoogleAccountProxy updates the proxy URL for an account.
func (d *DB) UpdateGoogleAccountProxy(id, proxy string) error {
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	encProxy, err := d.encryptField(proxy)
	if err != nil {
		return fmt.Errorf("failed to encrypt proxy: %w", err)
	}
	query := `UPDATE google_accounts SET proxy = ?, updated_at = ? WHERE id = ?`

	res, err := d.conn.Exec(query, encProxy, now, id)
	if err != nil {
		return fmt.Errorf("failed to update proxy for account %s: %w", id, err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return errors.New("google account not found")
	}

	return nil
}

// UpdateGoogleAccountFeatures updates image_enabled and video_enabled flags in SQLite.
func (d *DB) UpdateGoogleAccountFeatures(id string, imageEnabled, videoEnabled bool) error {
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	imgVal := 0
	if imageEnabled {
		imgVal = 1
	}
	vidVal := 0
	if videoEnabled {
		vidVal = 1
	}
	query := `UPDATE google_accounts SET image_enabled = ?, video_enabled = ?, updated_at = ? WHERE id = ?`
	_, err := d.conn.Exec(query, imgVal, vidVal, now, id)
	return err
}

// BulkUpdateGoogleAccountFeatures updates image_enabled or video_enabled for all accounts.
func (d *DB) BulkUpdateGoogleAccountFeatures(feature string, enabled bool) error {
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	val := 0
	if enabled {
		val = 1
	}
	switch feature {
	case "image":
		query := `UPDATE google_accounts SET image_enabled = ?, updated_at = ?`
		_, err := d.conn.Exec(query, val, now)
		return err
	case "video":
		query := `UPDATE google_accounts SET video_enabled = ?, updated_at = ?`
		_, err := d.conn.Exec(query, val, now)
		return err
	default:
		query := `UPDATE google_accounts SET image_enabled = ?, video_enabled = ?, updated_at = ?`
		_, err := d.conn.Exec(query, val, val, now)
		return err
	}
}

// DeleteGoogleAccount permanently removes an account record by ID.
func (d *DB) DeleteGoogleAccount(id string) error {
	query := `DELETE FROM google_accounts WHERE id = ?`
	res, err := d.conn.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete google account %s: %w", id, err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return errors.New("google account not found")
	}

	return nil
}

func normalizeTierValue(raw string) string {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "PRO":
		return "PRO"
	case "ULTRA":
		return "ULTRA"
	default:
		return "FREE"
	}
}
