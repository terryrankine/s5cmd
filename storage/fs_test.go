package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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

// The path up to the first wildcard is literal, as it is for S3: a directory
// named "data[2024]" is not a character class. From the wildcard on, the
// filepath.Glob syntax applies.
func TestFilesystemGlobLiteralPrefix(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	files := []string{
		"data[2024]/a.txt",
		"data[2024]/b.log",
		"data[2024]/sub/c.txt",
		"data2/a.txt",
		"plain/x[1].txt",
		"plain/x1.txt",
	}
	for _, name := range files {
		path := filepath.Join(tmpDir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	testcases := []struct {
		pattern string
		want    []string
	}{
		{
			// a glob would read [2024] as "one of 2, 0, 4" and match "data2"
			pattern: "data[2024]/*",
			want:    []string{"data[2024]/a.txt", "data[2024]/b.log", "data[2024]/sub"},
		},
		{
			pattern: "data[2024]/*.txt",
			want:    []string{"data[2024]/a.txt"},
		},
		{
			pattern: "data[2024]/*/*.txt",
			want:    []string{"data[2024]/sub/c.txt"},
		},
		{
			pattern: "data[2024]/?.txt",
			want:    []string{"data[2024]/a.txt"},
		},
		{
			// only "*" and "?" are wildcards, but from the first one on the
			// glob syntax applies, so this [1] still is a character class.
			pattern: "plain/x[1]*.txt",
			want:    []string{"plain/x1.txt"},
		},
		{
			pattern: "*/a.txt",
			want:    []string{"data2/a.txt", "data[2024]/a.txt"},
		},
		{
			pattern: "data[2024]/missing*",
			want:    nil,
		},
		{
			pattern: "missing[dir]/*",
			want:    nil,
		},
	}

	for _, tc := range testcases {
		tc := tc
		t.Run(tc.pattern, func(t *testing.T) {
			t.Parallel()

			u, err := url.New(filepath.Join(tmpDir, tc.pattern))
			if err != nil {
				t.Fatal(err)
			}
			if !u.IsWildcard() {
				t.Fatalf("%q is not a wildcard url", tc.pattern)
			}

			got, err := glob(u)
			if err != nil {
				t.Fatalf("glob failed: %v", err)
			}

			var want []string
			for _, name := range tc.want {
				want = append(want, filepath.Join(tmpDir, name))
			}
			if fmt.Sprint(got) != fmt.Sprint(want) {
				t.Errorf("glob(%q)\n got: %v\nwant: %v", tc.pattern, got, want)
			}
		})
	}
}

// A relative pattern is matched under the working directory, and the results
// keep the form of the pattern.
func TestFilesystemGlobRelative(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, "dir[1]"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "dir[1]", "f.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "g.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	// chdir is process-wide, so this test must not run in parallel.
	t.Chdir(tmpDir)

	testcases := []struct {
		pattern string
		want    []string
	}{
		{pattern: "*.txt", want: []string{"g.txt"}},
		{pattern: "./*.txt", want: []string{"g.txt"}},
		{pattern: "dir[1]/*", want: []string{filepath.Join("dir[1]", "f.txt")}},
		{pattern: "./dir[1]/*", want: []string{filepath.Join("dir[1]", "f.txt")}},
		{pattern: "*/f.txt", want: []string{filepath.Join("dir[1]", "f.txt")}},
	}

	for _, tc := range testcases {
		u, err := url.New(tc.pattern)
		if err != nil {
			t.Fatal(err)
		}
		got, err := glob(u)
		if err != nil {
			t.Fatalf("glob(%q) failed: %v", tc.pattern, err)
		}
		if fmt.Sprint(got) != fmt.Sprint(tc.want) {
			t.Errorf("glob(%q)\n got: %v\nwant: %v", tc.pattern, got, tc.want)
		}
	}
}
