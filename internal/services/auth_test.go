package services_test

import (
	"os"
	"path/filepath"
	"testing"

	"dezuxk/internal/db"
	"dezuxk/internal/services"
)

func TestAuthService(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_gateway.db")

	database, err := db.InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer database.Close()

	authService := services.NewAuthService(database)

	if _, err := authService.Register("admin", "1234"); err == nil {
		t.Fatal("expected a four-character password to be rejected")
	}

	// Test 1: First-run setup creates the only local administrator.
	adminUser, err := authService.Register("admin", "admin123456789")
	if err != nil {
		t.Fatalf("Initial admin setup failed: %v", err)
	}
	if adminUser.Username != "admin" {
		t.Errorf("Expected username 'admin', got '%s'", adminUser.Username)
	}

	// Test 2: Invalid password login
	_, err = authService.Login("admin", "wrongpassword")
	if err == nil {
		t.Errorf("Expected error for wrong password, got nil")
	}

	// Test 3: Change password
	err = authService.ChangePassword(adminUser.ID, "wrongpassword", "newsecret123")
	if err == nil {
		t.Errorf("Expected error when current password is wrong, got nil")
	}

	err = authService.ChangePassword(adminUser.ID, "admin123456789", "newsecret123")
	if err != nil {
		t.Fatalf("ChangePassword failed: %v", err)
	}

	// Verify login with old password fails
	_, err = authService.Login("", "admin123456789")
	if err == nil {
		t.Errorf("Expected old password to fail, but succeeded")
	}

	// Verify login with new password succeeds
	updatedUser, err := authService.Login("", "newsecret123")
	if err != nil {
		t.Fatalf("Login with new password failed: %v", err)
	}
	if updatedUser.ID != adminUser.ID {
		t.Errorf("Expected same user ID %d, got %d", adminUser.ID, updatedUser.ID)
	}

	if err := authService.ChangePassword(adminUser.ID, "newsecret123", "abcde"); err != nil {
		t.Fatalf("ChangePassword should accept a five-character password: %v", err)
	}
	if _, err := authService.Login("", "abcde"); err != nil {
		t.Fatalf("Login with a five-character password failed: %v", err)
	}

	// Test 4: Further registration is disabled after first-run setup.
	if _, err := authService.Register("developer", "devpass123456"); err == nil {
		t.Fatal("expected registration to be rejected after first-run setup")
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
