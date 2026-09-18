package command

import (
	"flag"
	"strings"
	"testing"

	"github.com/kballard/go-shellquote"
	"github.com/urfave/cli/v2"
	"pgregory.net/rapid"

	"github.com/peak/s5cmd/v2/storage/url"
)

// Any object key must survive the trip through the command line that sync
// generates and run parses: quotes, spaces, "$", newlines, control
// characters and bytes that are not UTF-8 have all broken it before
// (upstream #521, #728, #751).
func TestPropertyGeneratedCommandRoundTripsAnyKey(t *testing.T) {
	app := cli.NewApp()
	app.Commands = Commands()
	cmd := AppCommand("cp")

	rapid.Check(t, func(rt *rapid.T) {
		key := string(rapid.SliceOfN(rapid.Byte().Filter(func(c byte) bool { return c != 0 }), 1, 48).Draw(rt, "key"))
		if strings.ContainsAny(key, "*?") {
			rt.Skip("wildcard characters are patterns, not keys")
		}

		src, err := url.New("s3://bucket/"+key, url.WithRaw(true))
		if err != nil {
			rt.Fatalf("New: %v", err)
		}
		dst, err := url.New("dest/"+key, url.WithRaw(true))
		if err != nil {
			rt.Fatalf("New: %v", err)
		}

		set := flag.NewFlagSet(cmd.Name, flag.ContinueOnError)
		for _, f := range cmd.Flags {
			if err := f.Apply(set); err != nil {
				rt.Fatal(err)
			}
		}
		ctx := cli.NewContext(app, set, nil)

		line, err := generateCommand(ctx, cmd.Name, map[string]interface{}{"raw": true}, src, dst)
		if err != nil {
			rt.Fatalf("generateCommand: %v", err)
		}

		// this is what the run command does with the line
		fields, err := shellquote.Split(line)
		if err != nil {
			rt.Fatalf("run cannot parse %q: %v", line, err)
		}
		if len(fields) < 3 {
			rt.Fatalf("parsed %q into %d fields", line, len(fields))
		}
		gotSrc, gotDst := fields[len(fields)-2], fields[len(fields)-1]
		if gotSrc != src.String() {
			rt.Fatalf("source %q came back as %q", src.String(), gotSrc)
		}
		if gotDst != dst.String() {
			rt.Fatalf("destination %q came back as %q", dst.String(), gotDst)
		}
	})
}
