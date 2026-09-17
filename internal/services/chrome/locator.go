package chrome

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// FindChromeExecutable searches for Google Chrome on the system.
// Priority:
// 1. Env CHROME_PATH
// 2. Standard Windows 64-bit and 32-bit install paths
// 3. User LocalAppData install path
// 4. Windows Registry query (App Paths)
// 5. System PATH via exec.LookPath
func FindChromeExecutable() (string, error) {
	// 1. Environment variable override
	if envPath := strings.TrimSpace(os.Getenv("CHROME_PATH")); envPath != "" {
		if fileExists(envPath) {
			return filepath.Clean(envPath), nil
		}
	}

	// 2. Standard paths on Windows
	programFiles := os.Getenv("ProgramFiles")
	if programFiles == "" {
		programFiles = `C:\Program Files`
	}
	programFilesX86 := os.Getenv("ProgramFiles(x86)")
	if programFilesX86 == "" {
		programFilesX86 = `C:\Program Files (x86)`
	}
	localAppData := os.Getenv("LocalAppData")

	candidates := []string{
		filepath.Join(programFiles, "Google", "Chrome", "Application", "chrome.exe"),
		filepath.Join(programFilesX86, "Google", "Chrome", "Application", "chrome.exe"),
	}
	if localAppData != "" {
		candidates = append(candidates, filepath.Join(localAppData, "Google", "Chrome", "Application", "chrome.exe"))
	}

	for _, path := range candidates {
		if fileExists(path) {
			return filepath.Clean(path), nil
		}
	}

	// 3. Query Windows Registry for chrome.exe App Paths
	if regPath, err := queryRegistryAppPath("HKLM"); err == nil && fileExists(regPath) {
		return filepath.Clean(regPath), nil
	}
	if regPath, err := queryRegistryAppPath("HKCU"); err == nil && fileExists(regPath) {
		return filepath.Clean(regPath), nil
	}

	// 4. LookPath in system PATH
	if lp, err := exec.LookPath("chrome.exe"); err == nil && fileExists(lp) {
		return filepath.Clean(lp), nil
	}
	if lp, err := exec.LookPath("chrome"); err == nil && fileExists(lp) {
		return filepath.Clean(lp), nil
	}

	// Fallback to Edge if Chrome is truly not installed
	edgePath := filepath.Join(programFilesX86, "Microsoft", "Edge", "Application", "msedge.exe")
	if fileExists(edgePath) {
		return filepath.Clean(edgePath), nil
	}

	return "", errors.New("không tìm thấy Google Chrome trên hệ thống. Vui lòng cài đặt Google Chrome")
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func queryRegistryAppPath(hive string) (string, error) {
	cmd := exec.Command("reg", "query", hive+`\SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths\chrome.exe`, "/ve")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}

	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "REG_SZ") {
			parts := strings.SplitN(trimmed, "REG_SZ", 2)
			if len(parts) == 2 {
				val := strings.TrimSpace(parts[1])
				if fileExists(val) {
					return val, nil
				}
			}
		}
	}

	return "", errors.New("not found in registry")
}
