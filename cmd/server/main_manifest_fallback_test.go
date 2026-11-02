package main_test

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestServerEntryPoint_BrokenManifestHandling checks whether cmd/server/main.go
// fails fast or silently falls back when the specified manifests directory contains a broken JSON manifest.
func TestServerEntryPoint_BrokenManifestHandling(t *testing.T) {
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
	serverBin := filepath.Join(tmpDir, "server_bin")
	if runtime.GOOS == "windows" {
		serverBin += ".exe"
	}

	if out, err := exec.Command(goExe, "build", "-o", serverBin, "dezuxk/cmd/server").CombinedOutput(); err != nil {
		t.Fatalf("failed to compile server binary: %v\nOutput: %s", err, string(out))
	}

	dataDir := filepath.Join(tmpDir, "data")
	_ = os.MkdirAll(dataDir, 0o700)

	// Create a manifests dir containing an invalid JSON manifest
	manifestsDir := filepath.Join(tmpDir, "manifests")
	_ = os.MkdirAll(manifestsDir, 0o700)
	_ = os.WriteFile(filepath.Join(manifestsDir, "broken_flow.json"), []byte(`{ broken json file `), 0644)

	port := getFreePort(t)
	host := "127.0.0.1"

	serverCmd := exec.Command(
		serverBin,
		fmt.Sprintf("--host=%s", host),
		fmt.Sprintf("--port=%d", port),
		fmt.Sprintf("--data-dir=%s", dataDir),
		fmt.Sprintf("--manifests-dir=%s", manifestsDir),
		"--db-file=dezuxk_test.db",
	)

	outPipe, err := serverCmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	serverCmd.Stderr = serverCmd.Stdout

	if err := serverCmd.Start(); err != nil {
		t.Fatal(err)
	}

	var logs strings.Builder
	done := make(chan struct{})
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := outPipe.Read(buf)
			if n > 0 {
				logs.Write(buf[:n])
			}
			if err != nil {
				break
			}
		}
		close(done)
	}()

	// Poll /health
	client := &http.Client{Timeout: 500 * time.Millisecond}
	deadline := time.Now().Add(5 * time.Second)
	started := false
	for time.Now().Before(deadline) {
		resp, err := client.Get(fmt.Sprintf("http://%s:%d/health", host, port))
		if err == nil && resp.StatusCode == http.StatusOK {
			started = true
			resp.Body.Close()
			break
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(100 * time.Millisecond)
	}

	_ = serverCmd.Process.Kill()
	_ = serverCmd.Wait()
	<-done

	logStr := logs.String()
	t.Logf("Server logs:\n%s", logStr)

	if started {
		t.Logf("Empirical finding: server started successfully despite broken manifest in manifests-dir because main.go swallowed the error: err=%v and loaded embedded fallback.", logStr)
	}
}
