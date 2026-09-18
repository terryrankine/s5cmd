package url

import (
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// anyKey draws an object key of arbitrary bytes: keys are not required to be
// valid UTF-8, and file names with Latin-1 bytes, newlines or control
// characters have all broken s5cmd before (upstream #751).
func anyKey(t *rapid.T) string {
	b := rapid.SliceOfN(rapid.Byte().Filter(func(c byte) bool { return c != 0 }), 1, 48).Draw(t, "key")
	return string(b)
}

// Any key placed under a prefix must be found by a listing of that prefix,
// wildcard or not: New must accept it, and Match must accept the key.
func TestPropertyNewAndMatchAcceptAnyKey(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		key := anyKey(rt)
		if strings.ContainsAny(key, "*?") {
			// a wildcard in the key is a pattern, not a key: covered below
			rt.Skip("key contains a wildcard character")
		}

		for _, src := range []string{"s3://bucket/" + key, "s3://bucket/*", "s3://bucket/p/*"} {
			u, err := New(src)
			if err != nil {
				rt.Fatalf("New(%q): %v", src, err)
			}
			full := key
			if strings.HasPrefix(src, "s3://bucket/p/") {
				full = "p/" + key
			}
			if !u.Match(full) {
				rt.Fatalf("New(%q).Match(%q) = false", src, full)
			}
		}
	})
}

// A wildcard source must always compile: whatever bytes surround the "*"
// or "?", they are quoted, not interpreted.
func TestPropertyWildcardSourcesCompile(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		prefix := anyKey(rt)
		suffix := anyKey(rt)
		src := "s3://bucket/" + prefix + "*" + suffix
		u, err := New(src)
		if err != nil {
			rt.Fatalf("New(%q): %v", src, err)
		}
		if !u.IsWildcard() {
			rt.Fatalf("New(%q) is not a wildcard", src)
		}
		if !strings.ContainsAny(prefix+suffix, "*?") && !u.Match(prefix+"anything"+suffix) {
			rt.Fatalf("New(%q) does not match %q", src, prefix+"anything"+suffix)
		}
	})
}

// JoinInside never produces a path outside the destination: for any key it
// either fails or returns a path strictly under the destination directory.
func TestPropertyJoinInsideNeverEscapes(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		dest := rapid.SampledFrom([]string{"dest", "dest/", "a/b/dest", "/tmp/dest", ".", "/"}).Draw(rt, "dest")
		key := anyKey(rt)

		u, err := New(dest)
		if err != nil {
			rt.Fatalf("New(%q): %v", dest, err)
		}
		joined, err := u.JoinInside(key)
		if err != nil {
			return // rejected: fine
		}
		base := strings.TrimSuffix(u.Path, "/")
		got := joined.Path
		switch base {
		case ".":
			if got == "." || got == ".." || strings.HasPrefix(got, "../") || strings.HasPrefix(got, "/") {
				rt.Fatalf("JoinInside(%q, %q) = %q escapes", dest, key, got)
			}
		case "":
			// root: anything absolute is inside "/"
			if !strings.HasPrefix(got, "/") || got == "/" {
				rt.Fatalf("JoinInside(%q, %q) = %q escapes", dest, key, got)
			}
		default:
			if !strings.HasPrefix(got, base+"/") || got == base {
				rt.Fatalf("JoinInside(%q, %q) = %q escapes %q", dest, key, got, base)
			}
		}
	})
}

// A URL survives the extsort byte round-trip with its path, bucket and
// relative path intact, whatever bytes the key holds.
func TestPropertyURLBytesRoundTrip(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		key := anyKey(rt)
		if strings.ContainsAny(key, "*?") {
			rt.Skip("wildcard keys are patterns, not listed objects")
		}
		src, err := New("s3://bucket/p/*")
		if err != nil {
			rt.Fatal(err)
		}
		obj := src.Clone()
		obj.Path = "p/" + key
		if !src.Match(obj.Path) {
			rt.Fatalf("listing does not match %q", obj.Path)
		}
		obj.SetRelative(src)

		back := FromBytes(obj.ToBytes())
		if back.Path != obj.Path || back.Bucket != obj.Bucket || back.Type != obj.Type {
			rt.Fatalf("round trip changed %+v into %+v", obj, back)
		}
		if back.Relative() != obj.Relative() {
			rt.Fatalf("relative path %q became %q", obj.Relative(), back.Relative())
		}
	})
}
