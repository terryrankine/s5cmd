package command

import (
	"errors"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go/aws/awserr"
)

func TestShouldStopSync(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		err         error
		exitOnError bool
		want        bool
	}{
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
			name: "RequestError",
			err:  awserr.New("RequestError", "request error", nil),
			want: true,
		},
		{
			name: "SerializationError",
			err:  awserr.New("SerializationError", "serialization error", nil),
			want: true,
		},
		{
			name: "SlowDown should not stop",
			err:  awserr.New("SlowDown", "slow down", nil),
			want: false,
		},
		{
			name:        "non-AWS error with exitOnError true",
			err:         fmt.Errorf("some random error"),
			exitOnError: true,
			want:        true,
		},
		{
			name:        "non-AWS error with exitOnError false",
			err:         fmt.Errorf("some random error"),
			exitOnError: false,
			want:        false,
		},
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := Sync{
				exitOnError: tc.exitOnError,
			}

			got := s.shouldStopSync(tc.err)
			if got != tc.want {
				t.Errorf("shouldStopSync(%v) = %v, want %v", tc.err, got, tc.want)
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
