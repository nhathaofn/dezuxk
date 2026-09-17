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

	// Test 1: Default admin login with empty username (auto admin lookup)
	adminUser, err := authService.Login("", "admin123")
	if err != nil {
		t.Fatalf("Default admin login failed: %v", err)
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

	err = authService.ChangePassword(adminUser.ID, "admin123", "newsecret123")
	if err != nil {
		t.Fatalf("ChangePassword failed: %v", err)
	}

	// Verify login with old password fails
	_, err = authService.Login("", "admin123")
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

	// Test 4: Register new user
	newUser, err := authService.Register("developer", "devpass123")
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	if newUser.Username != "developer" {
		t.Errorf("Expected username 'developer', got '%s'", newUser.Username)
	}

	// Test 5: Login with newly registered user
	loggedInNewUser, err := authService.Login("developer", "devpass123")
	if err != nil {
		t.Fatalf("Login with registered user failed: %v", err)
	}
	if loggedInNewUser.ID != newUser.ID {
		t.Errorf("Expected ID %d, got %d", newUser.ID, loggedInNewUser.ID)
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
