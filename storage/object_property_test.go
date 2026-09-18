package storage

import (
	"os"
	"strings"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/peak/s5cmd/v2/storage/url"
)

// Every object sync lists goes through ToBytes/FromBytes in the external
// sort. Whatever the key holds, the object must come back equal in the
// fields sync compares on: path, size, modification instant and type.
func TestPropertyObjectBytesRoundTrip(t *testing.T) {
	src, err := url.New("s3://bucket/p/*")
	if err != nil {
		t.Fatal(err)
	}

	rapid.Check(t, func(rt *rapid.T) {
		key := string(rapid.SliceOfN(rapid.Byte().Filter(func(c byte) bool { return c != 0 }), 1, 48).Draw(rt, "key"))
		if strings.ContainsAny(key, "*?") {
			rt.Skip("wildcard characters are patterns, not keys")
		}

		u := src.Clone()
		u.Path = "p/" + key
		if !src.Match(u.Path) {
			rt.Fatalf("listing does not match %q", u.Path)
		}
		u.SetRelative(src)

		mod := time.Unix(rapid.Int64Range(0, 4102444800).Draw(rt, "sec"), rapid.Int64Range(0, 999999999).Draw(rt, "nsec")).UTC()
		isDir := rapid.Bool().Draw(rt, "dir")
		var mode os.FileMode
		if isDir {
			mode = os.ModeDir
		}
		obj := Object{
			URL:     u,
			ModTime: &mod,
			Size:    rapid.Int64Range(0, 1<<40).Draw(rt, "size"),
			Type:    ObjectType{mode},
		}

		data, err := obj.ToBytes()
		if err != nil {
			rt.Fatalf("ToBytes: %v", err)
		}
		back, err := FromBytes(data)
		if err != nil {
			rt.Fatalf("FromBytes: %v", err)
		}

		if back.URL.Path != obj.URL.Path || back.URL.Relative() != obj.URL.Relative() {
			rt.Fatalf("url %q/%q became %q/%q", obj.URL.Path, obj.URL.Relative(), back.URL.Path, back.URL.Relative())
		}
		if back.Size != obj.Size {
			rt.Fatalf("size %d became %d", obj.Size, back.Size)
		}
		if !back.ModTime.Equal(*obj.ModTime) {
			rt.Fatalf("modtime %v became %v", obj.ModTime, back.ModTime)
		}
		if back.Type.IsDir() != obj.Type.IsDir() {
			rt.Fatalf("type dir=%v became dir=%v", obj.Type.IsDir(), back.Type.IsDir())
		}
		if Compare(obj, back) != 0 {
			rt.Fatalf("Compare(obj, roundtrip) = %d", Compare(obj, back))
		}
	})
}
