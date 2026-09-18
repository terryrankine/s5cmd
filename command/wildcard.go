package command

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/urfave/cli/v2"

	"github.com/peak/s5cmd/v2/storage"
	"github.com/peak/s5cmd/v2/strutil"
)

// readPatternFile reads wildcard patterns from the file at path, one per
// line. Blank lines and lines starting with '#' are skipped, and leading and
// trailing whitespace is trimmed from every pattern.
func readPatternFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var patterns []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("%v: %w", path, err)
	}
	return patterns, nil
}

// patternsFromContext returns the patterns given with --<name>, followed by
// the patterns read from every file given with --<name>-from.
func patternsFromContext(c *cli.Context, name string) ([]string, error) {
	patterns := c.StringSlice(name)
	for _, path := range c.StringSlice(name + "-from") {
		filePatterns, err := readPatternFile(path)
		if err != nil {
			return nil, fmt.Errorf("--%s-from: %w", name, err)
		}
		patterns = append(patterns, filePatterns...)
	}
	return patterns, nil
}

// createRegexFromWildcard creates regex strings from wildcard.
func createRegexFromWildcard(wildcards []string) ([]*regexp.Regexp, error) {
	var result []*regexp.Regexp
	for _, input := range wildcards {
		if input != "" {
			regex := strutil.WildCardToRegexp(input)
			regex = strutil.MatchFromStartToEnd(regex)
			regex = strutil.AddNewLineFlag(regex)
			regexpCompiled, err := regexp.Compile(regex)
			if err != nil {
				return nil, err
			}
			result = append(result, regexpCompiled)
		}
	}
	return result, nil
}

func isURLMatched(regexPatterns []*regexp.Regexp, urlPath, sourcePrefix string) bool {
	if len(regexPatterns) == 0 {
		return false
	}
	if !strings.HasSuffix(sourcePrefix, "/") {
		sourcePrefix += "/"
	}
	sourcePrefix = filepath.ToSlash(sourcePrefix)
	for _, regexPattern := range regexPatterns {
		if regexPattern.MatchString(strings.TrimPrefix(urlPath, sourcePrefix)) {
			return true
		}
	}
	return false
}

func isObjectExcluded(object *storage.Object, excludePatterns []*regexp.Regexp, includePatterns []*regexp.Regexp, prefix string) (bool, error) {
	if err := object.Err; err != nil {
		return true, err
	}
	if len(excludePatterns) > 0 && isURLMatched(excludePatterns, object.URL.Path, prefix) {
		return true, nil
	}
	if len(includePatterns) > 0 {
		return !isURLMatched(includePatterns, object.URL.Path, prefix), nil
	}
	return false, nil
}
