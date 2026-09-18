//go:build !windows

package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/peak/s5cmd/v2/storage/url"
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

// On Unix a backslash is an ordinary character in a file name. Before the
// first wildcard it must stay one, not become a filepath.Glob escape.
func TestFilesystemGlobBackslashInPrefix(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	dir := filepath.Join(tmpDir, `back\slash`)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	u, err := url.New(filepath.Join(dir, "*"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := glob(u)
	if err != nil {
		t.Fatalf("glob failed: %v", err)
	}
	want := []string{filepath.Join(dir, "f.txt")}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("glob(%q)\n got: %v\nwant: %v", u.Absolute(), got, want)
	}
}
