//go:build !windows

package security

// Unix-like filesystems use the mode supplied by the caller. Windows uses the
// platform ACL implementation in storage_acl_windows.go.
func HardenFile(string) error      { return nil }
func HardenDirectory(string) error { return nil }
func HardenDatabase(string) error  { return nil }
