package command

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go/aws/awserr"

	"github.com/peak/s5cmd/v2/storage"
)

func TestIsListingError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
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
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := isListingError(tc.err)
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
