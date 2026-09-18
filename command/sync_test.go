package command

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/google/go-cmp/cmp"
	"github.com/kballard/go-shellquote"
	"github.com/urfave/cli/v2"

	"github.com/peak/s5cmd/v2/storage"
	"github.com/peak/s5cmd/v2/storage/url"
)

func TestIsListingError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		url  *url.URL
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "empty S3 listing is not an error",
			err:  storage.ErrNoObjectFound,
			want: false,
		},
		{
			name: "local wildcard without a match is an empty listing",
			err:  &storage.ErrNoMatchFound{Pattern: "dir/*"},
			want: false,
		},
		{
			name: "cancellation is not a listing error",
			err:  context.Canceled,
			want: false,
		},
		{
			name: "AccessDenied",
			err:  awserr.New("AccessDenied", "access denied", nil),
			want: true,
		},
		{
			name: "NoSuchBucket",
			err:  awserr.New("NoSuchBucket", "no such bucket", nil),
			want: true,
		},
		{
			name: "BucketRegionError",
			err:  awserr.New("BucketRegionError", "incorrect region", nil),
			want: true,
		},
		{
			name: "RequestError",
			err:  awserr.New("RequestError", "request error", nil),
			want: true,
		},
		{
			name: "SlowDown after retries are exhausted",
			err:  awserr.New("SlowDown", "slow down", nil),
			want: true,
		},
		{
			name: "non-AWS error",
			err:  fmt.Errorf("lstat dangling: no such file or directory"),
			want: true,
		},
		{
			name: "an error about one listed object is not a listing error",
			err:  &storage.ErrGivenObjectNotFound{ObjectAbsPath: "dir/dangling"},
			url:  mustNewURL(t, "dir/dangling"),
			want: false,
		},
		{
			name: "an unreadable directory leaves the listing incomplete",
			err:  fmt.Errorf("open dir/locked: permission denied"),
			url:  mustNewURL(t, "dir/locked/"),
			want: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := isListingError(&storage.Object{URL: tc.url, Err: tc.err})
			if got != tc.want {
				t.Errorf("isListingError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestSyncErrors(t *testing.T) {
	t.Parallel()

	first := fmt.Errorf("first")
	second := fmt.Errorf("second")

	tests := []struct {
		name string
		errs []error
		want string
	}{
		{name: "none", errs: nil, want: ""},
		{name: "one", errs: []error{first}, want: "first"},
		{name: "many", errs: []error{first, second, second}, want: "first (and 2 more errors)"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var e syncErrors
			for _, err := range tc.errs {
				e.add(err)
			}

			got := e.err()
			if tc.want == "" {
				if got != nil {
					t.Fatalf("err() = %v, want nil", got)
				}
				return
			}
			if got == nil || got.Error() != tc.want {
				t.Errorf("err() = %v, want %q", got, tc.want)
			}
			if !errors.Is(got, first) {
				t.Errorf("err() = %v does not wrap the first error", got)
			}
		})
	}
}

// TestSyncPlanRunDeleteBatches checks that the destination-only objects are
// written as rm commands of at most syncDeleteBatchSize URLs each, as they
// arrive, instead of one rm command holding every URL: that command, and
// the goroutine rm starts per URL, is what made a large sync --delete run
// out of memory (upstream peak/s5cmd#745).
func TestSyncPlanRunDeleteBatches(t *testing.T) {
	t.Parallel()

	const n = 2*syncDeleteBatchSize + 1

	tests := []struct {
		name       string
		cancel     bool
		skippedSrc int
		wantLines  int
		wantErrs   int
	}{
		{name: "batches", wantLines: 3},
		// a cancelled sync deletes nothing, not even the batch it holds.
		{name: "cancelled", cancel: true, wantLines: 0},
		// a source object skipped with an error is missing from the
		// comparison, so nothing may be deleted (upstream peak/s5cmd#800).
		{name: "skipped source", skippedSrc: 1, wantLines: 0, wantErrs: 1},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := Sync{op: "sync", delete: true, errs: &syncErrors{}}
			for i := 0; i < tc.skippedSrc; i++ {
				s.errs.addSkippedSrc()
			}
			dsturl := mustNewURL(t, "s3://bucket/prefix/")

			onlySource := make(chan *url.URL)
			common := make(chan *ObjectPair)
			close(onlySource)
			close(common)

			var want []string
			onlyDest := make(chan *url.URL, n)
			for i := 0; i < n; i++ {
				u := mustNewURL(t, fmt.Sprintf("s3://bucket/prefix/obj-%05d", i))
				want = append(want, u.String())
				onlyDest <- u
			}
			close(onlyDest)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.cancel {
				cancel()
			}

			set := flagSet(t, "sync", NewSyncCommandFlags())
			cliCtx := cli.NewContext(app, set, nil)

			pr, pw := io.Pipe()
			go s.planRun(ctx, cliCtx, onlySource, onlyDest, common, dsturl, NewStrategy(false), pw, true)

			out, err := io.ReadAll(pr)
			if err != nil {
				t.Fatal(err)
			}

			lines := strings.Split(strings.TrimSpace(string(out)), "\n")
			if len(out) == 0 {
				lines = nil
			}
			if len(lines) != tc.wantLines {
				t.Fatalf("got %d command lines, want %d:\n%s", len(lines), tc.wantLines, out)
			}
			if s.errs.count != tc.wantErrs {
				t.Errorf("got %d reported errors, want %d: %v", s.errs.count, tc.wantErrs, s.errs.first)
			}
			if tc.skippedSrc > 0 {
				return
			}

			var got []string
			for _, line := range lines {
				fields, err := shellquote.Split(line)
				if err != nil {
					t.Fatal(err)
				}
				if len(fields) < 2 || fields[0] != "rm" || fields[1] != "--raw=true" {
					t.Fatalf("unexpected command %q", line)
				}
				urls := fields[2:]
				if len(urls) > syncDeleteBatchSize {
					t.Errorf("rm command has %d urls, want at most %d", len(urls), syncDeleteBatchSize)
				}
				got = append(got, urls...)
			}
			if tc.cancel {
				return
			}
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("deleted urls (-want +got):\n%v", diff)
			}
		})
	}
}
