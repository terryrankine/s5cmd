package storage

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestFilesystemImplementsStorageInterface(t *testing.T) {
	var i interface{} = new(Filesystem)
	if _, ok := i.(Storage); !ok {
		t.Errorf("expected %t to implement Storage interface", i)
	}
}

func TestFilesystemCreateDryRun(t *testing.T) {
	t.Parallel()

	fs := &Filesystem{dryRun: true}
	tmpDir := t.TempDir()

	testcases := []struct {
		name   string
		create func() (*os.File, error)
	}{
		{
			name: "Create",
			create: func() (*os.File, error) {
				return fs.Create(filepath.Join(tmpDir, "should-not-exist.txt"))
			},
		},
		{
			name: "CreateTemp",
			create: func() (*os.File, error) {
				return fs.CreateTemp(tmpDir, "dryrun-*.txt")
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			f, err := tc.create()
			if err != nil {
				t.Fatalf("%s() returned error: %v", tc.name, err)
			}
			if f == nil {
				t.Fatalf("%s() returned nil file", tc.name)
			}

			if f.Name() != os.DevNull {
				t.Fatalf("expected file name %q, got %q", os.DevNull, f.Name())
			}

			// the handle must not alias stdin, otherwise closing it would
			// close stdin as well.
			if f.Fd() == 0 {
				t.Fatal("expected file descriptor other than stdin")
			}

			if _, err := f.Write([]byte("content")); err != nil {
				t.Fatalf("Write() returned error: %v", err)
			}

			if err := f.Close(); err != nil {
				t.Fatalf("Close() returned error: %v", err)
			}

			if _, err := os.Stdin.Stat(); err != nil {
				t.Fatalf("stdin is not usable after Close(): %v", err)
			}
		})
	}

	// no file should be created on disk during dry run
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no files in temp dir during dry run, got %d", len(entries))
	}
}

func TestFilesystemCreateTempNaming(t *testing.T) {
	t.Parallel()

	fs := &Filesystem{}
	tmpDir := t.TempDir()

	testcases := []struct {
		name    string
		pattern string
		prefix  string
		suffix  string
	}{
		{name: "no wildcard", pattern: "file.txt", prefix: "file.txt", suffix: ""},
		{name: "wildcard in middle", pattern: "file-*.txt", prefix: "file-", suffix: ".txt"},
		{name: "wildcard at end", pattern: "file.txt.*", prefix: "file.txt.", suffix: ""},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			f, err := fs.CreateTemp(tmpDir, tc.pattern)
			if err != nil {
				t.Fatalf("CreateTemp() returned error: %v", err)
			}
			defer f.Close()

			if got := filepath.Dir(f.Name()); got != tmpDir {
				t.Fatalf("expected file in %q, got %q", tmpDir, got)
			}

			base := filepath.Base(f.Name())
			if !strings.HasPrefix(base, tc.prefix) || !strings.HasSuffix(base, tc.suffix) {
				t.Fatalf("expected name %q<random>%q, got %q", tc.prefix, tc.suffix, base)
			}

			random := strings.TrimSuffix(strings.TrimPrefix(base, tc.prefix), tc.suffix)
			if _, err := strconv.ParseUint(random, 10, 32); err != nil {
				t.Fatalf("expected a decimal random part in %q, got %q", base, random)
			}

			// the file must be usable for both reading and writing
			if _, err := f.Write([]byte("content")); err != nil {
				t.Fatalf("Write() returned error: %v", err)
			}
			if _, err := f.ReadAt(make([]byte, 1), 0); err != nil {
				t.Fatalf("ReadAt() returned error: %v", err)
			}
		})
	}

	t.Run("pattern with separator", func(t *testing.T) {
		_, err := fs.CreateTemp(tmpDir, "dir"+string(os.PathSeparator)+"file")
		if err == nil {
			t.Fatal("expected error for pattern containing a path separator")
		}
	})

	t.Run("unique names", func(t *testing.T) {
		seen := make(map[string]struct{})
		for i := 0; i < 20; i++ {
			f, err := fs.CreateTemp(tmpDir, "unique-*")
			if err != nil {
				t.Fatalf("CreateTemp() returned error: %v", err)
			}
			f.Close()
			if _, ok := seen[f.Name()]; ok {
				t.Fatalf("CreateTemp() returned the same name twice: %q", f.Name())
			}
			seen[f.Name()] = struct{}{}
		}
	})
}
