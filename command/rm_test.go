package command

import (
	"testing"

	"github.com/urfave/cli/v2"
)

func TestValidateRMCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		sources        []string
		raw            bool
		expectedErrStr string
	}{
		{
			name: "error_if_local_and_remote_mixed",
			sources: []string{
				"s3://bucket/key",
				"filename.txt",
			},
			expectedErrStr: "arguments cannot have both local and remote sources",
		},
		{
			name: "error_if_sources_have_bucket",
			sources: []string{
				"s3://bucket/key",
				"s3://bucket",
			},
			expectedErrStr: "s3 bucket/prefix cannot be used for delete operations (forgot wildcard character?)",
		},
		{
			name: "error_if_sources_have_s3_prefix",
			sources: []string{
				"s3://bucket/prefix/",
			},
			expectedErrStr: "s3 bucket/prefix cannot be used for delete operations (forgot wildcard character?)",
		},
		{
			name: "success",
			sources: []string{
				"s3://bucket/prefix/filename.txt",
				"s3://bucket/wildcard/*.txt",
			},
		},
		{
			name: "error_if_sources_have_s3_prefix_with_raw_flag_and_bucket",
			sources: []string{
				"s3://bucket",
			},
			raw:            true,
			expectedErrStr: "s3 bucket/prefix cannot be used for delete operations (forgot wildcard character?)",
		},
		{
			name: "success_if_sources_have_s3_prefix_with_raw_flag",
			sources: []string{
				"s3://bucket/prefix/",
			},
			raw: true,
		},
		{
			name: "error_if_different_buckets",
			sources: []string{
				"s3://bucket/object",
				"s3://someotherbucket/object",
			},
			expectedErrStr: "removal of objects with different buckets in a single command is not allowed",
		},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			cmd := NewDeleteCommand()
			flagset := flagSet(t, cmd.Name, cmd.Flags)
			if tc.raw {
				if err := flagset.Set("raw", "true"); err != nil {
					t.Fatal(err)
				}
			}
			if err := flagset.Parse(tc.sources); err != nil {
				t.Error(err)
			}
			ctx := cli.NewContext(app, flagset, nil)

			err := validateRMCommand(ctx)
			if (err != nil && err.Error() != tc.expectedErrStr) ||
				(err == nil && tc.expectedErrStr != "") {
				t.Errorf("expected_got = %v, error_expected = %v", err, tc.expectedErrStr)
			}
		})
	}
}
