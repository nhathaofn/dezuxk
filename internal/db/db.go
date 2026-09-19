package db

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"dezuxk/internal/config"
	"dezuxk/internal/crypto"
	"dezuxk/internal/models"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

// DB wraps the database connection.
type DB struct {
	conn            *sql.DB
	cipherKey       []byte
	legacyCipherKey []byte
}

// CipherKey returns the current encryption key.
func (d *DB) CipherKey() []byte {
	return d.cipherKey
}

// SetCipherKey sets the encryption key (useful for tests).
func (d *DB) SetCipherKey(k []byte) {
	d.cipherKey = k
}

// InitDB initializes SQLite database connection, applies migrations and seeds default data.
func InitDB(dbPath string) (*DB, error) {
	if dbPath == "" {
		dbPath = config.DefaultDBPath
	}

	dir := filepath.Dir(dbPath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, fmt.Errorf("failed to create database directory: %w", err)
		}
	}

	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	// Enable WAL mode for better concurrency and integrity
	if _, err := conn.Exec("PRAGMA journal_mode=WAL; PRAGMA foreign_keys = ON;"); err != nil {
		log.Printf("Warning: failed to set pragma on sqlite: %v", err)
	}

	db := &DB{conn: conn}

	if err := db.migrate(); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("migration failed: %w", err)
	}

	if err := db.seedDefaultAdmin(); err != nil {
		log.Printf("Warning: failed to seed default admin: %v", err)
	}

	// Initialize machine-bound encryption key for sensitive data protection
	key, err := crypto.GetOrInitMasterKey(db)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("failed to initialize encryption key: %w", err)
	}
	db.cipherKey = key
	if legacyKey, legacyErr := crypto.GetLegacyMasterKey(db); legacyErr == nil {
		// Keep this only in memory while upgrading records encrypted by older
		// versions. New writes always use the current DPAPI-protected key.
		db.legacyCipherKey = legacyKey
	}

	// Convert any legacy plaintext session material while the database key is available.
	// Google passwords/recovery emails are intentionally removed; the application never
	// needs them to maintain a browser session.
	if err := db.migrateSensitiveFields(); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("sensitive-data migration failed: %w", err)
	}

	return db, nil
}

// Checkpoint flushes WAL pages to the primary database file.
func (d *DB) Checkpoint(truncate bool) error {
	if d.conn == nil {
		return nil
	}
	cmd := "PRAGMA wal_checkpoint(PASSIVE);"
	if truncate {
		cmd = "PRAGMA wal_checkpoint(TRUNCATE);"
	}
	_, err := d.conn.Exec(cmd)
	return err
}

// Close closes the underlying database connection after checkpointing WAL.
func (d *DB) Close() error {
	if d.conn != nil {
		_ = d.Checkpoint(true)
		err := d.conn.Close()
		for i := range d.cipherKey {
			d.cipherKey[i] = 0
		}
		for i := range d.legacyCipherKey {
			d.legacyCipherKey[i] = 0
		}
		return err
	}
	return nil
}

// migrate creates necessary tables if they do not exist.
func (d *DB) migrate() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			role TEXT NOT NULL DEFAULT 'admin',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS google_accounts (
			id TEXT PRIMARY KEY,
			email TEXT NOT NULL,
			name TEXT NOT NULL DEFAULT '',
			avatar_url TEXT NOT NULL DEFAULT '',
			profile_dir TEXT NOT NULL,
			cookies TEXT NOT NULL,
			snlm0e_token TEXT NOT NULL DEFAULT '',
			services TEXT NOT NULL DEFAULT 'gemini,flow',
			status TEXT NOT NULL DEFAULT 'ACTIVE',
			last_error TEXT,
			last_refresh_at DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
	}

	for _, q := range queries {
		if _, err := d.conn.Exec(q); err != nil {
			return err
		}
	}

	// Migrate optional columns if not present
	d.addColumnIfNotExists("google_accounts", "tier", "TEXT DEFAULT 'FREE'")
	d.addColumnIfNotExists("google_accounts", "credits", "INTEGER DEFAULT 0")
	d.addColumnIfNotExists("google_accounts", "proxy", "TEXT DEFAULT ''")
	d.addColumnIfNotExists("google_accounts", "image_enabled", "INTEGER DEFAULT 1")
	d.addColumnIfNotExists("google_accounts", "video_enabled", "INTEGER DEFAULT 1")
	d.addColumnIfNotExists("google_accounts", "user_agent", "TEXT DEFAULT ''")
	d.addColumnIfNotExists("google_accounts", "password", "TEXT DEFAULT ''")
	d.addColumnIfNotExists("google_accounts", "recovery_email", "TEXT DEFAULT ''")
	if _, err := d.conn.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_google_accounts_email
		ON google_accounts(email) WHERE email <> ''`); err != nil {
		return fmt.Errorf("failed to enforce unique Google account email: %w", err)
	}

	return nil
}

// addColumnIfNotExists adds a column to a table if it does not already exist.
func (d *DB) addColumnIfNotExists(table, column, colType string) {
	query := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s;", table, column, colType)
	_, _ = d.conn.Exec(query)
}

// seedDefaultAdmin seeds a default admin account if no users exist.
func (d *DB) seedDefaultAdmin() error {
	var count int
	err := d.conn.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	if err != nil {
		return err
	}

	if count == 0 {
		initialPassword := strings.TrimSpace(os.Getenv(config.AdminPasswordEnv))
		if initialPassword == "" {
			log.Printf("[DB] No admin user exists. Complete first-run setup to create one.")
			return nil
		}
		if len(initialPassword) < config.MinPasswordLength {
			return fmt.Errorf("%s must contain at least 5 characters", config.AdminPasswordEnv)
		}

		hashed, err := bcrypt.GenerateFromPassword([]byte(initialPassword), bcrypt.DefaultCost)
		if err != nil {
			return err
		}

		_, err = d.conn.Exec(
			"INSERT INTO users (username, password_hash, role, created_at) VALUES (?, ?, ?, ?)",
			config.DefaultAdminUser,
			string(hashed),
			"admin",
			time.Now().UTC().Format("2006-01-02 15:04:05"),
		)
		if err != nil {
			return err
		}
		log.Println("[DB] Initialized the configured admin account in SQLite.")
	}

	return nil
}

// CountUsers returns the number of local application users.
func (d *DB) CountUsers() (int, error) {
	var count int
	if err := d.conn.QueryRow("SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

// GetSetting retrieves a configuration value from SQLite by key.
func (d *DB) GetSetting(key, defaultValue string) string {
	var val string
	err := d.conn.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&val)
	if err != nil {
		return defaultValue
	}
	return val
}

// SetSetting stores or updates a configuration value in SQLite.
func (d *DB) SetSetting(key, value string) error {
	_, err := d.conn.Exec(
		"INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value",
		key,
		value,
	)
	return err
}

// GetAdminUser returns the primary admin user account.
func (d *DB) GetAdminUser() (*models.User, error) {
	row := d.conn.QueryRow(
		"SELECT id, username, password_hash, role, created_at FROM users WHERE role = 'admin' ORDER BY id ASC LIMIT 1",
	)

	var u models.User
	var createdAtStr string
	err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &createdAtStr)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("admin user not found")
		}
		return nil, err
	}

	u.CreatedAt = createdAtStr
	return &u, nil
}

// UpdateUserPassword updates the password hash for a specific user.
func (d *DB) UpdateUserPassword(userID int64, newPasswordHash string) error {
	res, err := d.conn.Exec(
		"UPDATE users SET password_hash = ? WHERE id = ?",
		newPasswordHash,
		userID,
	)
	if err != nil {
		return err
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return errors.New("user not found")
	}

	return nil
}

// GetUserByUsername finds a user by their username.
func (d *DB) GetUserByUsername(username string) (*models.User, error) {
	row := d.conn.QueryRow(
		"SELECT id, username, password_hash, role, created_at FROM users WHERE username = ?",
		username,
	)

	var u models.User
	var createdAtStr string
	err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &createdAtStr)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("user not found")
		}
		return nil, err
	}

	u.CreatedAt = createdAtStr
	return &u, nil
}

// CreateUser inserts a new user with the given credentials.
func (d *DB) CreateUser(username, passwordHash, role string) (*models.User, error) {
	now := time.Now().UTC()
	formattedNow := now.Format("2006-01-02 15:04:05")
	res, err := d.conn.Exec(
		"INSERT INTO users (username, password_hash, role, created_at) VALUES (?, ?, ?, ?)",
		username,
		passwordHash,
		role,
		formattedNow,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to insert user: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &models.User{
		ID:           id,
		Username:     username,
		PasswordHash: passwordHash,
		Role:         role,
		CreatedAt:    formattedNow,
	}, nil
}

// GetUserByID finds a user by their ID.
func (d *DB) GetUserByID(id int64) (*models.User, error) {
	row := d.conn.QueryRow(
		"SELECT id, username, password_hash, role, created_at FROM users WHERE id = ?",
		id,
	)

	var u models.User
	var createdAtStr string
	err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &createdAtStr)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("user not found")
		}
		return nil, err
	}

	u.CreatedAt = createdAtStr
	return &u, nil
}
