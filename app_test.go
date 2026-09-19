package main

import (
	"testing"

	"dezuxk/internal/db"
	"dezuxk/internal/services"
)

func TestAppGoogleOperationsRequireBackendAdminSession(t *testing.T) {
	database, err := db.InitDB(t.TempDir() + "\\gateway.db")
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer database.Close()

	app := &App{
		database:    database,
		authService: services.NewAuthService(database),
	}
	app.accountService = services.NewAccountService(database, t.TempDir())

	if _, err := app.ListGoogleAccounts(); err == nil {
		t.Fatal("unauthenticated account list should be rejected")
	}
	if !app.IsAuthSetupRequired() {
		t.Fatal("empty database should require first-run setup")
	}

	if _, err := app.Register("admin", "test-admin-password"); err != nil {
		t.Fatalf("initial setup failed: %v", err)
	}
	if app.IsAuthSetupRequired() {
		t.Fatal("setup should be complete after creating the first admin")
	}
	if _, err := app.ListGoogleAccounts(); err != nil {
		t.Fatalf("authenticated account list failed: %v", err)
	}

	app.Logout()
	if _, err := app.ListGoogleAccounts(); err == nil {
		t.Fatal("account list should be rejected after logout")
	}
}
