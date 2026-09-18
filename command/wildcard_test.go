package command

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/peak/s5cmd/v2/storage"
	"github.com/peak/s5cmd/v2/storage/url"
	"gotest.tools/v3/assert"
)

func TestIsObjectExcluded(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		excludePatterns []string
		includePatterns []string
		objects         []string
		filteredObjects []string
	}{
		{
			excludePatterns: []string{"*.txt", "*.log"},
			includePatterns: []string{"file-*.doc"},
			objects:         []string{"document.txt", "file-2.log", "file-1.doc", "image.png"},
			filteredObjects: []string{"file-1.doc"},
		},
		{
			excludePatterns: []string{"secret-*"},
			includePatterns: []string{"*.txt", "*.log"},
			objects:         []string{"secret-passwords.txt", "file-1.txt", "file-2.txt", "image.png"},
			filteredObjects: []string{"file-1.txt", "file-2.txt"},
		},
		{
			excludePatterns: []string{},
			includePatterns: []string{"*.png"},
			objects:         []string{"secret-passwords.txt", "file-1.txt", "file-2.txt", "image.png"},
			filteredObjects: []string{"image.png"},
		},
		{
			excludePatterns: []string{"file*"},
			includePatterns: []string{},
			objects:         []string{"readme.md", "file-1.txt", "file-2.txt", "image.png"},
			filteredObjects: []string{"readme.md", "image.png"},
		},
		{
			excludePatterns: []string{"file*"},
			includePatterns: []string{"*txt"},
			objects:         []string{"readme.txt", "file-1.txt", "file-2.txt", "license.txt"},
			filteredObjects: []string{"readme.txt", "license.txt"},
		},
		{
			excludePatterns: []string{"*tmp", "*.txt"},
			includePatterns: []string{"*png", "*.doc*"},
			objects:         []string{"readme.txt", "license.txt", "cache.tmp", "image.png", "eula.doc", "eula.docx", "personaldoc"},
			filteredObjects: []string{"image.png", "eula.doc", "eula.docx"},
		},
	}

	for _, tc := range testcases {
		tc := tc

		excludeRegex, err := createRegexFromWildcard(tc.excludePatterns)
		if err != nil {
			t.Error(err)
		}

		includeRegex, err := createRegexFromWildcard(tc.includePatterns)
		if err != nil {
			t.Error(err)
		}

		var filteredObjects []string

		for _, object := range tc.objects {
			skip, err := isObjectExcluded(&storage.Object{URL: &url.URL{Path: object}}, excludeRegex, includeRegex, "")
			if err != nil {
				t.Fatal(err)
			}
			if skip {
				continue
			}
			filteredObjects = append(filteredObjects, object)
		}

		assert.DeepEqual(t, tc.filteredObjects, filteredObjects)
	}
}

func TestReadPatternFile(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		name     string
		content  string
		expected []string
	}{
		{
			name:     "one pattern per line",
			content:  "*.txt\nlog*\n",
			expected: []string{"*.txt", "log*"},
		},
		{
			name:     "no trailing newline",
			content:  "*.txt",
			expected: []string{"*.txt"},
		},
		{
			name:     "blank lines and comments are skipped",
			content:  "# excluded patterns\n\n*.txt\n   \n  # indented comment\nlog*\n",
			expected: []string{"*.txt", "log*"},
		},
		{
			name:     "surrounding whitespace is trimmed",
			content:  "  *.txt  \n\tlog*\t\r\n",
			expected: []string{"*.txt", "log*"},
		},
		{
			name:     "empty file",
			content:  "",
			expected: nil,
		},
	}

	for _, tc := range testcases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "patterns.txt")
			if err := os.WriteFile(path, []byte(tc.content), 0o600); err != nil {
				t.Fatal(err)
			}

			got, err := readPatternFile(path)
			if err != nil {
				t.Fatal(err)
			}

			assert.DeepEqual(t, tc.expected, got)
		})
	}
}

func TestReadPatternFileMissing(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "missing.txt")

	got, err := readPatternFile(path)
	assert.Assert(t, os.IsNotExist(err), "expected not-exist error, got %v", err)
	assert.Assert(t, got == nil)
}
