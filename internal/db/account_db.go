package db

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"dezuxk/internal/models"
)

// CreateGoogleAccount inserts a new Google account into the database.
func (d *DB) CreateGoogleAccount(acc *models.GoogleAccount) error {
	if acc == nil || acc.ID == "" {
		return errors.New("invalid account data")
	}

	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	if acc.CreatedAt == "" {
		acc.CreatedAt = now
	}
	acc.UpdatedAt = now

	if acc.Tier == "" {
		acc.Tier = "FREE"
	}

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

	_, err := d.conn.Exec(
		query,
		acc.ID,
		acc.Email,
		acc.Name,
		acc.AvatarURL,
		acc.ProfileDir,
		acc.Cookies,
		acc.SNlM0eToken,
		acc.Services,
		acc.Status,
		acc.Tier,
		acc.Credits,
		acc.Proxy,
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

	existing, err := d.GetGoogleAccountByEmail(acc.Email)
	if err == nil && existing != nil {
		// Update existing record
		now := time.Now().UTC().Format("2006-01-02 15:04:05")
		acc.ID = existing.ID
		if acc.ProfileDir == "" {
			acc.ProfileDir = existing.ProfileDir
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
			last_refresh_at = ?,
			updated_at = ?
		WHERE id = ?`

		_, err = d.conn.Exec(
			query,
			acc.Cookies,
			acc.SNlM0eToken, acc.SNlM0eToken,
			acc.Tier, acc.Tier,
			acc.Credits, acc.Credits,
			acc.Proxy, acc.Proxy,
			acc.Status,
			acc.Services,
			acc.ProfileDir,
			now,
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

	return &acc, nil
}

// GetGoogleAccountByEmail retrieves an account by email address.
func (d *DB) GetGoogleAccountByEmail(email string) (*models.GoogleAccount, error) {
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

	res, err := d.conn.Exec(query, cookies, snlm0e, snlm0e, credits, credits, tier, tier, now, now, id)
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
	query := `UPDATE google_accounts SET proxy = ?, updated_at = ? WHERE id = ?`

	res, err := d.conn.Exec(query, proxy, now, id)
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
	if feature == "image" {
		query := `UPDATE google_accounts SET image_enabled = ?, updated_at = ?`
		_, err := d.conn.Exec(query, val, now)
		return err
	} else if feature == "video" {
		query := `UPDATE google_accounts SET video_enabled = ?, updated_at = ?`
		_, err := d.conn.Exec(query, val, now)
		return err
	}
	query := `UPDATE google_accounts SET image_enabled = ?, video_enabled = ?, updated_at = ?`
	_, err := d.conn.Exec(query, val, val, now)
	return err
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

