package progressbar

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/assert"
)

func TestCommandProgress_Finish(t *testing.T) {
	t.Parallel()
	cp := New()
	cp.Start()
	cp.Finish()
	assert.Equal(t, true, cp.progressbar.IsFinished())
}

func TestCommandProgress_IncrementCompletedObjects(t *testing.T) {
	t.Parallel()
	cp := New()
	cp.IncrementCompletedObjects()
	assert.Equal(t, int64(1), cp.completedObjects)
	assert.Equal(t, true, strings.Contains(cp.progressbar.String(), "1/0"))
}

func TestCommandProgress_IncrementTotalObjects(t *testing.T) {
	t.Parallel()
	cp := New()
	cp.Start()
	cp.IncrementTotalObjects()
	assert.Equal(t, int64(1), cp.totalObjects)
	assert.Equal(t, true, strings.Contains(cp.progressbar.String(), "0/1"))
}

func TestCommandProgress_AddCompletedBytes(t *testing.T) {
	t.Parallel()
	cp := New()
	cp.Start()
	bytes := int64(101)
	cp.AddCompletedBytes(bytes)
	assert.Equal(t, bytes, cp.progressbar.Current())
	assert.Equal(t, true, strings.Contains(cp.progressbar.String(), "101 B"))
}

func TestCommandProgress_AddTotalBytes(t *testing.T) {
	t.Parallel()
	cp := New()
	cp.Start()
	bytes := int64(102)
	cp.AddTotalBytes(bytes)
	assert.Equal(t, bytes, cp.progressbar.Total())
	assert.Equal(t, true, strings.Contains(cp.progressbar.String(), "102 B"))
}

func TestFormatDuration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input time.Duration
		want  string
	}{
		{5 * time.Second, "5s"},
		{65 * time.Second, "1m5s"},
		{3661 * time.Second, "1h1m1s"},
		{0, "0s"},
	}

	for _, tc := range tests {
		got := formatDuration(tc.input)
		assert.Equal(t, tc.want, got)
	}
}

func TestLogProgress_FinishNoObjects(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	lp := NewLogProgressBar()
	lp.out = &buf
	lp.Start()
	lp.Finish()
	assert.Equal(t, "", buf.String())
}

func TestLogProgress_FinishWithObjects(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	lp := NewLogProgressBar()
	lp.out = &buf
	lp.Start()
	lp.IncrementTotalObjects()
	lp.AddTotalBytes(1536)
	lp.IncrementCompletedObjects()
	lp.AddCompletedBytes(1536)
	lp.Finish()

	lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	assert.Equal(t, 2, len(lines))
	assert.Equal(t, true, strings.HasPrefix(lines[0], "100.0% - 1.5K/1.5K @ "), lines[0])
	assert.Equal(t, true, strings.HasSuffix(lines[0], " (1/1 files)"), lines[0])
	assert.Equal(t, "Transfer complete", lines[1])
}
