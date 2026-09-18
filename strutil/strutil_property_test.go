package strutil

import (
	"regexp"
	"testing"
	"unicode/utf8"

	"pgregory.net/rapid"
)

// QuoteMeta must turn any byte string into a pattern that compiles and
// matches exactly that string, valid UTF-8 or not (upstream #751).
func TestPropertyQuoteMetaCompilesAndMatchesItself(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		b := rapid.SliceOfN(rapid.Byte(), 0, 64).Draw(rt, "bytes")
		s := string(b)

		re, err := regexp.Compile("^" + QuoteMeta(s) + "$")
		if err != nil {
			rt.Fatalf("QuoteMeta(%q) does not compile: %v", s, err)
		}
		if !re.MatchString(s) {
			rt.Fatalf("QuoteMeta(%q) does not match its own input", s)
		}
	})
}

// A wildcard pattern built from arbitrary bytes must compile, and "*" must
// match any bytes in that position.
func TestPropertyWildCardToRegexpMatchesItsPrefix(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		prefix := string(rapid.SliceOfN(rapid.Byte().Filter(func(c byte) bool { return c != '*' && c != '?' }), 0, 24).Draw(rt, "prefix"))
		filler := string(rapid.SliceOfN(rapid.Byte(), 0, 24).Draw(rt, "filler"))
		// Go's regexp sees an invalid byte as U+FFFD, so a pattern cannot tell
		// a lone 0xC2 from the first byte of U+0080: a prefix that ends inside
		// a multi-byte sequence cannot match input where the sequence goes on.
		// That is a limit of the engine, not a bug in the quoting.
		if r, _ := utf8.DecodeLastRuneInString(prefix); r == utf8.RuneError && prefix != "" && filler != "" && filler[0]&0xC0 == 0x80 {
			rt.Skip("prefix ends inside a multi-byte sequence that the filler continues")
		}

		re, err := regexp.Compile(WildCardToRegexp(prefix + "*"))
		if err != nil {
			rt.Fatalf("WildCardToRegexp(%q): %v", prefix+"*", err)
		}
		if !re.MatchString(prefix + filler) {
			rt.Fatalf("%q* does not match %q", prefix, prefix+filler)
		}
	})
}
