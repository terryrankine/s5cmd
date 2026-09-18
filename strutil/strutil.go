package strutil

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

var humanDivisors = [...]struct {
	suffix string
	div    int64
}{
	{"K", 1 << 10},
	{"M", 1 << 20},
	{"G", 1 << 30},
	{"T", 1 << 40},
}

// HumanizeBytes takes a byte-size and returns a human-readable string
func HumanizeBytes(b int64) string {
	var (
		suffix string
		div    int64
	)
	for _, f := range humanDivisors {
		if b > f.div {
			suffix = f.suffix
			div = f.div
		}
	}
	if suffix == "" {
		return strconv.FormatInt(b, 10) + "B"
	}

	return fmt.Sprintf("%.1f%s", float64(b)/float64(div), suffix)
}

// JSON is a helper function for creating JSON-encoded strings.
func JSON(v interface{}) string {
	bytes, _ := json.Marshal(v)
	return string(bytes)
}

// CapitalizeFirstRune converts first rune to uppercase, and converts rest of
// the string to lower case.
func CapitalizeFirstRune(str string) string {
	if str == "" {
		return str
	}
	runes := []rune(str)
	first, rest := runes[0], runes[1:]
	return strings.ToUpper(string(first)) + strings.ToLower(string(rest))
}

// AddNewLineFlag adds a flag that allows . to match new line character "\n".
// It assumes that the pattern does not have any flags.
func AddNewLineFlag(pattern string) string {
	return "(?s)" + pattern
}

// QuoteMeta is regexp.QuoteMeta for text that may not be valid UTF-8, such
// as a file name written by a program that used another encoding. regexp
// rejects a pattern with invalid UTF-8 in it, yet matches each invalid byte
// of its input as U+FFFD; so that is what each invalid byte becomes.
func QuoteMeta(s string) string {
	s = regexp.QuoteMeta(s)
	if utf8.ValidString(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			b.WriteString(`\x{FFFD}`)
		} else {
			b.WriteString(s[i : i+size])
		}
		i += size
	}
	return b.String()
}

// WildCardToRegexp converts a wildcarded expresiion to equivalent regular expression
func WildCardToRegexp(pattern string) string {
	patternRegex := QuoteMeta(pattern)
	patternRegex = strings.Replace(patternRegex, "\\?", ".", -1)
	return strings.Replace(patternRegex, "\\*", ".*", -1)
}

// MatchFromStartToEnd enforces that the regex will match the full string
func MatchFromStartToEnd(pattern string) string {
	return "^" + pattern + "$"
}
