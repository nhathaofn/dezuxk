//go:build windows

package security

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
)

// HardenFile removes inherited broad permissions from a sensitive application file.
func HardenFile(path string) error {
	principal, err := currentPrincipal()
	if err != nil {
		return err
	}
	return runICACLS(path,
		"/inheritance:r",
		"/grant:r", principal+":F", "*S-1-5-18:F", "*S-1-5-32-544:F",
	)
}

// HardenDatabase protects the SQLite database and any sidecar files that may
// contain live WAL or rollback-journal pages while the application is running.
func HardenDatabase(path string) error {
	paths := []string{path}
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		candidate := path + suffix
		if _, err := os.Stat(candidate); err == nil {
			paths = append(paths, candidate)
		}
	}
	for _, candidate := range paths {
		if err := HardenFile(candidate); err != nil {
			return err
		}
	}
	return nil
}

// HardenDirectory applies a private ACL to a data directory and its descendants.
func HardenDirectory(path string) error {
	principal, err := currentPrincipal()
	if err != nil {
		return err
	}
	return runICACLS(path,
		"/inheritance:r", "/T", "/C",
		"/grant:r", principal+":(OI)(CI)F", "*S-1-5-18:(OI)(CI)F", "*S-1-5-32-544:(OI)(CI)F",
	)
}

func currentPrincipal() (string, error) {
	username := os.Getenv("USERNAME")
	if username == "" {
		return "", errors.New("USERNAME is not available for ACL hardening")
	}
	if domain := os.Getenv("USERDOMAIN"); domain != "" {
		return domain + `\` + username, nil
	}
	if computer := os.Getenv("COMPUTERNAME"); computer != "" {
		return computer + `\` + username, nil
	}
	return username, nil
}

func runICACLS(path string, args ...string) error {
	cmdArgs := append([]string{path}, args...)
	if output, err := exec.Command("icacls", cmdArgs...).CombinedOutput(); err != nil {
		return fmt.Errorf("icacls failed for %s: %w (%s)", path, err, string(output))
	}
	return nil
}
