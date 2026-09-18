//go:build !windows

package storage

import (
	"os"
	"syscall"
	"testing"
)

// Downloaded files must honour the caller's umask, as os.Create does, and
// must not depend on a chmod after creation (some filesystems reject it).
func TestFilesystemCreateTempHonoursUmask(t *testing.T) {
	// umask is process-wide, so this test must not run in parallel.
	fs := &Filesystem{}
	tmpDir := t.TempDir()

	testcases := []struct {
		umask int
		mode  os.FileMode
	}{
		{umask: 0o022, mode: 0o644},
		{umask: 0o002, mode: 0o664},
		{umask: 0o077, mode: 0o600},
	}

	for _, tc := range testcases {
		old := syscall.Umask(tc.umask)
		f, err := fs.CreateTemp(tmpDir, "umask-*")
		syscall.Umask(old)

		if err != nil {
			t.Fatalf("CreateTemp() returned error: %v", err)
		}
		f.Close()

		st, err := os.Stat(f.Name())
		if err != nil {
			t.Fatalf("Stat() returned error: %v", err)
		}
		if got := st.Mode().Perm(); got != tc.mode {
			t.Errorf("umask %04o: expected mode %04o, got %04o", tc.umask, tc.mode, got)
		}
	}
}
