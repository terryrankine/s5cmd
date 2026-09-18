//go:build windows

package e2e

// setUmask is a no-op on Windows, which has no umask.
func setUmask() {}
