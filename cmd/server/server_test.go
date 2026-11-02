package main_test

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// getFreePort returns an available TCP port on localhost.
func getFreePort(t *testing.T) int {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to find free port: %v", err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

// TestServerEntryPoint_GracefulStartupWithoutMasterKey tests:
// 1. Graceful startup of cmd/server/main.go when NO master key is provided
// 2. Auto-generation of <DataDir>/.master.key (64-character hex)
// 3. Health check execution returning HTTP 200 and expected payload
// 4. Zero secret key leakage in stdout/stderr server logs
// 5. Clean graceful shutdown
// 6. Persistence across restart without re-generating key
func TestServerEntryPoint_GracefulStartupWithoutMasterKey(t *testing.T) {
	goExe := "go"
	if customGo := os.Getenv("GO_EXE"); customGo != "" {
		goExe = customGo
	} else if runtime.GOOS == "windows" {
		standardPath := `C:\Users\PC\AppData\Local\Programs\go1.27.3\go\bin\go.exe`
		if _, err := os.Stat(standardPath); err == nil {
			goExe = standardPath
		}
	}

	tmpDir := t.TempDir()
	serverBin := filepath.Join(tmpDir, "server_test_bin")
	if runtime.GOOS == "windows" {
		serverBin += ".exe"
	}

	// 1. Compile cmd/server/main.go
	buildCmd := exec.Command(goExe, "build", "-o", serverBin, ".")
	buildCmd.Dir = filepath.Dir(serverBin)
	// We are running within cmd/server during go test
	if out, err := exec.Command(goExe, "build", "-o", serverBin, "dezuxk/cmd/server").CombinedOutput(); err != nil {
		t.Fatalf("failed to compile server binary: %v\nOutput: %s", err, string(out))
	}

	dataDir := filepath.Join(tmpDir, "server_data")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatalf("failed to create data dir: %v", err)
	}

	port := getFreePort(t)
	host := "127.0.0.1"

	// 2. Launch server with NO master key, pointing to fresh data directory
	serverCmd := exec.Command(
		serverBin,
		fmt.Sprintf("--host=%s", host),
		fmt.Sprintf("--port=%d", port),
		fmt.Sprintf("--data-dir=%s", dataDir),
		"--db-file=dezuxk_test.db",
	)

	// Ensure no master key environment variable is inherited
	serverCmd.Env = append(os.Environ(),
		"DEZUXK_SECURITY_MASTER_KEY=",
		"DEZUXK_SECURITY_MASTER_KEY_FILE=",
	)

	// Capture stdout and stderr
	outPipe, err := serverCmd.StdoutPipe()
	if err != nil {
		t.Fatalf("failed to get stdout pipe: %v", err)
	}
	serverCmd.Stderr = serverCmd.Stdout // merge stderr into stdout

	if err := serverCmd.Start(); err != nil {
		t.Fatalf("failed to start server subprocess: %v", err)
	}

	// Read logs asynchronously
	var serverLogs strings.Builder
	doneReading := make(chan struct{})
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := outPipe.Read(buf)
			if n > 0 {
				serverLogs.Write(buf[:n])
			}
			if err != nil {
				break
			}
		}
		close(doneReading)
	}()

	// 3. Poll /health until server is ready (up to 10 seconds)
	healthURL := fmt.Sprintf("http://%s:%d/health", host, port)
	client := &http.Client{Timeout: 1 * time.Second}

	var healthResp *http.Response
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.Get(healthURL)
		if err == nil && resp.StatusCode == http.StatusOK {
			healthResp = resp
			break
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(100 * time.Millisecond)
	}

	if healthResp == nil {
		_ = serverCmd.Process.Kill()
		<-doneReading
		t.Fatalf("server failed to respond to /health on %s within deadline\nLogs:\n%s", healthURL, serverLogs.String())
	}
	defer healthResp.Body.Close()

	// 4. Validate health check payload
	body, err := io.ReadAll(healthResp.Body)
	if err != nil {
		t.Fatalf("failed to read health response body: %v", err)
	}

	var healthPayload struct {
		Status   string `json:"status"`
		Version  string `json:"version"`
		Accounts struct {
			Ready int `json:"ready"`
			Total int `json:"total"`
		} `json:"accounts"`
	}
	if err := json.Unmarshal(body, &healthPayload); err != nil {
		t.Fatalf("failed to parse /health response JSON %q: %v", string(body), err)
	}

	if healthPayload.Status != "healthy" {
		t.Errorf("expected health status 'healthy', got %q", healthPayload.Status)
	}

	// 5. Verify <DataDir>/.master.key file was created
	masterKeyFile := filepath.Join(dataDir, ".master.key")
	keyData, err := os.ReadFile(masterKeyFile)
	if err != nil {
		t.Fatalf("expected .master.key to be created at %s, but read failed: %v", masterKeyFile, err)
	}

	resolvedKey := strings.TrimSpace(string(keyData))
	if len(resolvedKey) != 64 {
		t.Errorf("expected 64 hex characters for master key, got len=%d (%q)", len(resolvedKey), resolvedKey)
	}
	rawBytes, err := hex.DecodeString(resolvedKey)
	if err != nil || len(rawBytes) != 32 {
		t.Errorf("master key is not 32 hex-encoded bytes: err=%v, len=%d", err, len(rawBytes))
	}

	// 6. Graceful shutdown
	if runtime.GOOS == "windows" {
		// On Windows, os.Interrupt is not always caught by subprocess signal handler,
		// but Process.Signal(os.Interrupt) or Process.Kill can be used.
		// Try Interrupt first
		_ = serverCmd.Process.Signal(os.Interrupt)
	} else {
		_ = serverCmd.Process.Signal(os.Interrupt)
	}

	// Wait with timeout
	exitCh := make(chan error, 1)
	go func() {
		exitCh <- serverCmd.Wait()
	}()

	select {
	case <-exitCh:
		// Exited
	case <-time.After(3 * time.Second):
		_ = serverCmd.Process.Kill()
		<-exitCh
	}
	<-doneReading

	logs := serverLogs.String()

	// 7. Verify zero secret leakage in logs
	if strings.Contains(logs, resolvedKey) {
		t.Errorf("CRITICAL SECURITY LEAK: Resolved master key leaked in server logs:\n%s", logs)
	}

	// Verify expected log messages
	if !strings.Contains(logs, "master key resolved") {
		t.Errorf("expected logs to contain 'master key resolved', got:\n%s", logs)
	}
	if !strings.Contains(logs, "auto_generated_persistent") {
		t.Errorf("expected logs to contain source 'auto_generated_persistent', got:\n%s", logs)
	}

	// 8. Restart check: verify restart reuses the existing .master.key without overwrite
	statBefore, err := os.Stat(masterKeyFile)
	if err != nil {
		t.Fatalf("stat before restart failed: %v", err)
	}

	port2 := getFreePort(t)
	serverCmd2 := exec.Command(
		serverBin,
		fmt.Sprintf("--host=%s", host),
		fmt.Sprintf("--port=%d", port2),
		fmt.Sprintf("--data-dir=%s", dataDir),
		"--db-file=dezuxk_test.db",
	)
	serverCmd2.Env = append(os.Environ(),
		"DEZUXK_SECURITY_MASTER_KEY=",
		"DEZUXK_SECURITY_MASTER_KEY_FILE=",
	)

	outPipe2, _ := serverCmd2.StdoutPipe()
	serverCmd2.Stderr = serverCmd2.Stdout
	if err := serverCmd2.Start(); err != nil {
		t.Fatalf("failed to start second server instance: %v", err)
	}

	var logs2 strings.Builder
	done2 := make(chan struct{})
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := outPipe2.Read(buf)
			if n > 0 {
				logs2.Write(buf[:n])
			}
			if err != nil {
				break
			}
		}
		close(done2)
	}()

	healthURL2 := fmt.Sprintf("http://%s:%d/health", host, port2)
	deadline2 := time.Now().Add(10 * time.Second)
	var healthResp2 *http.Response
	for time.Now().Before(deadline2) {
		resp, err := client.Get(healthURL2)
		if err == nil && resp.StatusCode == http.StatusOK {
			healthResp2 = resp
			break
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(100 * time.Millisecond)
	}

	if healthResp2 == nil {
		_ = serverCmd2.Process.Kill()
		<-done2
		t.Fatalf("second server instance failed to respond to /health: logs=\n%s", logs2.String())
	}
	healthResp2.Body.Close()

	// Shutdown second instance
	_ = serverCmd2.Process.Kill()
	_ = serverCmd2.Wait()
	<-done2

	statAfter, err := os.Stat(masterKeyFile)
	if err != nil {
		t.Fatalf("stat after restart failed: %v", err)
	}

	if !statBefore.ModTime().Equal(statAfter.ModTime()) {
		t.Errorf("expected .master.key to be preserved without modification on restart")
	}

	keyData2, err := os.ReadFile(masterKeyFile)
	if err != nil || strings.TrimSpace(string(keyData2)) != resolvedKey {
		t.Errorf("master key changed after restart: %s vs %s", string(keyData2), resolvedKey)
	}
}
