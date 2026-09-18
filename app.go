package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"dezuxk/internal/config"
	"dezuxk/internal/db"
	"dezuxk/internal/models"
	"dezuxk/internal/services"
)

// App struct holds application state and provides methods
// that can be called from the frontend via Wails bindings.
type App struct {
	ctx            context.Context
	mu             sync.RWMutex
	cfg            *config.AppConfig
	database       *db.DB
	authService    *services.AuthService
	gatewayService *services.GatewayService
	accountService *services.AccountService
	currentUser    *models.UserResponse
}

// NewApp creates a new App application struct.
func NewApp() *App {
	return &App{
		cfg: config.LoadConfig(),
	}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// Initialize SQLite database from config
	database, err := db.InitDB(a.cfg.DBPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	a.database = database
	a.authService = services.NewAuthService(database)
	a.accountService = services.NewAccountService(database, "data")
	a.accountService.StartBackgroundWorkers()
	log.Println("[App] SQLite database initialized and ready.")

	// Retrieve saved port from SQLite settings, fallback to config
	portStr := database.GetSetting(config.SettingGatewayPort, strconv.Itoa(a.cfg.GatewayPort))
	initialPort, err := strconv.Atoi(portStr)
	if err != nil || initialPort <= 0 || initialPort > 65535 {
		initialPort = a.cfg.GatewayPort
	}

	// Initialize and launch gateway server on saved port
	a.gatewayService = services.NewGatewayService(initialPort)
	if _, err := a.gatewayService.Start(initialPort); err != nil {
		log.Printf("[App] Gateway autostart warning: %v\n", err)
	}
}

// shutdown is called when the app is closing.
func (a *App) shutdown(ctx context.Context) {
	if a.accountService != nil {
		a.accountService.StopBackgroundWorkers()
	}
	if a.gatewayService != nil {
		_, _ = a.gatewayService.Stop()
	}
	if a.database != nil {
		if err := a.database.Close(); err != nil {
			log.Printf("Error closing database: %v", err)
		}
	}
}

// Login authenticates a user with username and password.
func (a *App) Login(username, password string) (*models.UserResponse, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	user, err := a.authService.Login(username, password)
	if err != nil {
		return nil, err
	}

	a.currentUser = user
	return user, nil
}

// ChangePassword changes the current user's password in SQLite.
func (a *App) ChangePassword(currentPassword, newPassword string) error {
	a.mu.RLock()
	var userID int64
	if a.currentUser != nil {
		userID = a.currentUser.ID
	}
	a.mu.RUnlock()

	return a.authService.ChangePassword(userID, currentPassword, newPassword)
}

// Register registers a new user with username and password.
func (a *App) Register(username, password string) (*models.UserResponse, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	user, err := a.authService.Register(username, password)
	if err != nil {
		return nil, err
	}

	a.currentUser = user
	return user, nil
}

// GetCurrentUser returns the currently logged in user, or nil.
func (a *App) GetCurrentUser() *models.UserResponse {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.currentUser
}

// Logout clears the current active user session.
func (a *App) Logout() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.currentUser = nil
	return true
}

// GetGatewayStatus returns the current status (running, IP, port) of the gateway server.
func (a *App) GetGatewayStatus() *models.GatewayStatus {
	if a.gatewayService == nil {
		return &models.GatewayStatus{IsRunning: false, IP: "127.0.0.1", Port: a.cfg.GatewayPort}
	}
	return a.gatewayService.GetStatus()
}

// ToggleGateway toggles the gateway server on or off.
func (a *App) ToggleGateway() (*models.GatewayStatus, error) {
	if a.gatewayService == nil {
		a.gatewayService = services.NewGatewayService(a.cfg.GatewayPort)
	}
	return a.gatewayService.Toggle()
}

// StartGateway starts the gateway server on a specified port.
func (a *App) StartGateway(port int) (*models.GatewayStatus, error) {
	if a.gatewayService == nil {
		a.gatewayService = services.NewGatewayService(port)
	}
	return a.gatewayService.Start(port)
}

// StopGateway stops the running gateway server.
func (a *App) StopGateway() (*models.GatewayStatus, error) {
	if a.gatewayService == nil {
		return &models.GatewayStatus{IsRunning: false, IP: "127.0.0.1", Port: a.cfg.GatewayPort}, nil
	}
	return a.gatewayService.Stop()
}

// GetSettings returns current application settings.
func (a *App) GetSettings() (*models.AppSettings, error) {
	if a.gatewayService == nil {
		return nil, errors.New("gateway service not initialized")
	}

	status := a.gatewayService.GetStatus()
	return &models.AppSettings{
		GatewayPort: status.Port,
		GatewayIP:   status.IP,
		IsRunning:   status.IsRunning,
	}, nil
}

// UpdateGatewayPort updates the configured port, persists to SQLite, and updates the server.
func (a *App) UpdateGatewayPort(port int) (*models.GatewayStatus, error) {
	if port <= 0 || port > 65535 {
		return nil, errors.New("Cổng không hợp lệ (hợp lệ từ 1024 đến 65535)")
	}

	// Persist to SQLite settings
	if a.database != nil {
		if err := a.database.SetSetting(config.SettingGatewayPort, strconv.Itoa(port)); err != nil {
			log.Printf("[App] Warning saving port to SQLite: %v\n", err)
		}
	}

	if a.gatewayService == nil {
		a.gatewayService = services.NewGatewayService(port)
		return a.gatewayService.GetStatus(), nil
	}

	return a.gatewayService.SetPort(port)
}

// ListGoogleAccounts returns all saved Google accounts in SQLite.
func (a *App) ListGoogleAccounts() ([]*models.GoogleAccountResponse, error) {
	if a.accountService == nil {
		return nil, errors.New("account service not initialized")
	}
	return a.accountService.ListAccounts()
}

// StartGoogleLogin initiates an interactive Chrome login session.
func (a *App) StartGoogleLogin(service, proxy string) (*models.LoginSessionStatus, error) {
	if a.accountService == nil {
		return nil, errors.New("account service not initialized")
	}
	return a.accountService.StartLogin(service, proxy)
}

// GetGoogleLoginStatus queries the current progress of an active login session.
func (a *App) GetGoogleLoginStatus(sessionID string) (*models.LoginSessionStatus, error) {
	if a.accountService == nil {
		return nil, errors.New("account service not initialized")
	}
	return a.accountService.GetLoginStatus(sessionID)
}

// CancelGoogleLogin terminates an ongoing login session.
func (a *App) CancelGoogleLogin(sessionID string) error {
	if a.accountService == nil {
		return errors.New("account service not initialized")
	}
	return a.accountService.CancelLogin(sessionID)
}

// DeleteGoogleAccount removes an account from the pool.
func (a *App) DeleteGoogleAccount(accountID string) error {
	if a.accountService == nil {
		return errors.New("account service not initialized")
	}
	return a.accountService.DeleteAccount(accountID)
}

// RefreshGoogleAccount manually triggers a headless refresh of an account's session tokens.
func (a *App) RefreshGoogleAccount(accountID string) (*models.GoogleAccountResponse, error) {
	if a.accountService == nil {
		return nil, errors.New("account service not initialized")
	}
	return a.accountService.RefreshSession(accountID)
}

// TestGoogleAccount verifies live connectivity of an account against Google.
func (a *App) TestGoogleAccount(accountID string) (*models.AccountTestResult, error) {
	if a.accountService == nil {
		return nil, errors.New("account service not initialized")
	}
	return a.accountService.TestSession(accountID)
}

// OpenGoogleAccountBrowser opens a dedicated Chrome browser window for the specified account.
func (a *App) OpenGoogleAccountBrowser(accountID string) error {
	if a.accountService == nil {
		return errors.New("account service not initialized")
	}
	return a.accountService.OpenAccountBrowser(accountID)
}

// ToggleGoogleAccount switches an account between ACTIVE and DISABLED.
func (a *App) ToggleGoogleAccount(accountID string, active bool) (*models.GoogleAccountResponse, error) {
	if a.accountService == nil {
		return nil, errors.New("account service not initialized")
	}
	return a.accountService.ToggleAccount(accountID, active)
}

// RefreshAllGoogleAccounts refreshes all enabled accounts.
func (a *App) RefreshAllGoogleAccounts() ([]*models.GoogleAccountResponse, error) {
	if a.accountService == nil {
		return nil, errors.New("account service not initialized")
	}
	return a.accountService.RefreshAllAccounts()
}

// AddGoogleAccountManual manually inserts an account with cookies and optional token/proxy.
func (a *App) AddGoogleAccountManual(input models.ManualAccountInput) (*models.GoogleAccountResponse, error) {
	if a.accountService == nil {
		return nil, errors.New("account service not initialized")
	}
	return a.accountService.AddAccountManual(input)
}

// BulkAddGoogleAccounts imports accounts from a multiline text list.
func (a *App) BulkAddGoogleAccounts(input models.BulkAddInput) (*models.BulkAddResult, error) {
	if a.accountService == nil {
		return nil, errors.New("account service not initialized")
	}
	return a.accountService.BulkAddAccounts(input)
}

// ExportAccountsBackupDialog prompts the user with a Windows Save File Dialog and creates a backup zip.
func (a *App) ExportAccountsBackupDialog() (*models.BackupExportResult, error) {
	if a.accountService == nil {
		return nil, errors.New("account service not initialized")
	}

	defaultFilename := fmt.Sprintf("glabs-flow-accounts-%s.zip", time.Now().Format("20060102-150405"))
	savePath, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "Sao lưu tài khoản Google & Flow ra file ZIP",
		DefaultFilename: defaultFilename,
		Filters: []runtime.FileFilter{
			{DisplayName: "Zip Archive (*.zip)", Pattern: "*.zip"},
		},
	})
	if err != nil {
		return nil, err
	}
	if savePath == "" {
		return nil, nil // User cancelled
	}

	return a.accountService.ExportBackup(savePath)
}

// ImportAccountsBackupDialog prompts the user with a Windows Open File Dialog to select any backup zip or accounts.json.
func (a *App) ImportAccountsBackupDialog() (*models.RestoreResult, error) {
	if a.accountService == nil {
		return nil, errors.New("account service not initialized")
	}

	filePath, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Chọn file sao lưu tài khoản G-Labs (*.zip hoặc accounts.json)",
		Filters: []runtime.FileFilter{
			{DisplayName: "G-Labs Backup Files (*.zip, accounts.json)", Pattern: "*.zip;accounts.json"},
			{DisplayName: "All Files (*.*)", Pattern: "*.*"},
		},
	})
	if err != nil {
		return nil, err
	}
	if filePath == "" {
		return nil, nil // User cancelled
	}

	return a.accountService.ImportGLabsBackup(filePath)
}

// ImportGLabsBackup imports accounts and profiles from a G-Labs backup folder or zip file.
func (a *App) ImportGLabsBackup(backupPath string) (*models.RestoreResult, error) {
	if a.accountService == nil {
		return nil, errors.New("account service not initialized")
	}
	return a.accountService.ImportGLabsBackup(backupPath)
}

// TestGoogleAccountProxy tests real connectivity, latency, and egress IP for a proxy string.
func (a *App) TestGoogleAccountProxy(proxy string) (*models.ProxyTestResult, error) {
	if a.accountService == nil {
		return nil, errors.New("account service not initialized")
	}
	return a.accountService.TestProxy(proxy)
}

// SaveGoogleAccountProxy updates the proxy URL for a Google account.
func (a *App) SaveGoogleAccountProxy(accountID, proxy string) error {
	if a.accountService == nil {
		return errors.New("account service not initialized")
	}
	return a.accountService.SaveAccountProxy(accountID, proxy)
}

// UpdateGoogleAccountFeatures updates image_enabled and video_enabled flags for an account.
func (a *App) UpdateGoogleAccountFeatures(accountID string, imageEnabled, videoEnabled bool) error {
	if a.accountService == nil {
		return errors.New("account service not initialized")
	}
	return a.accountService.UpdateAccountFeatures(accountID, imageEnabled, videoEnabled)
}

// BulkUpdateGoogleAccountFeatures updates feature flags for all accounts.
func (a *App) BulkUpdateGoogleAccountFeatures(feature string, enabled bool) error {
	if a.accountService == nil {
		return errors.New("account service not initialized")
	}
	return a.accountService.BulkUpdateFeatures(feature, enabled)
}

// UpdateGoogleAccountCredits updates the credit balance of an account directly.
func (a *App) UpdateGoogleAccountCredits(accountID string, credits int) error {
	if a.accountService == nil {
		return errors.New("account service not initialized")
	}
	return a.accountService.UpdateCredits(accountID, credits)
}






