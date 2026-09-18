package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/peak/s5cmd/v2/parallel"
	"github.com/peak/s5cmd/v2/storage/url"
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

// TestFilesystemMultiDelete checks that every file sent to MultiDelete is
// removed and reported exactly once, including failures, when the deletes
// run on the parallel manager.
func TestFilesystemMultiDelete(t *testing.T) {
	const numFiles = 50

	parallel.Init(4)
	defer parallel.Close()

	fs := &Filesystem{}
	tmpDir := t.TempDir()

	var paths []string
	for i := 0; i < numFiles; i++ {
		path := filepath.Join(tmpDir, fmt.Sprintf("file-%02d.txt", i))
		if err := os.WriteFile(path, []byte("content"), 0644); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}
		paths = append(paths, path)
	}
	missing := filepath.Join(tmpDir, "missing.txt")
	paths = append(paths, missing)

	urlch := make(chan *url.URL)
	go func() {
		defer close(urlch)
		for _, path := range paths {
			u, err := url.New(path)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			urlch <- u
		}
	}()

	seen := make(map[string]int)
	for obj := range fs.MultiDelete(context.Background(), urlch) {
		path := filepath.Clean(obj.URL.Absolute())
		seen[path]++

		if path == missing {
			if obj.Err == nil {
				t.Errorf("expected an error for %q", path)
			}
			continue
		}
		if obj.Err != nil {
			t.Errorf("unexpected error for %q: %v", path, obj.Err)
		}
	}

	if len(seen) != len(paths) {
		t.Errorf("expected %d results, got %d", len(paths), len(seen))
	}
	for _, path := range paths {
		if seen[path] != 1 {
			t.Errorf("expected %q to be reported once, got %d", path, seen[path])
		}
	}

	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected all files to be removed, %d left", len(entries))
	}
}
