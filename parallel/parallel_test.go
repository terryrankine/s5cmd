package parallel

import (
	"errors"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewWorkerCount(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		name  string
		count int
		want  int
	}{
		{name: "positive", count: 8, want: 8},
		{name: "below minimum", count: 1, want: minNumWorkers},
		{name: "zero", count: 0, want: minNumWorkers},
		{name: "negative is a multiple of NumCPU", count: -2, want: max(runtime.NumCPU()*2, minNumWorkers)},
	}

	for _, tc := range testcases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := cap(New(tc.count).semaphore); got != tc.want {
				t.Errorf("New(%d): got %d workers, want %d", tc.count, got, tc.want)
			}
		})
	}
}

func TestRunBoundsConcurrency(t *testing.T) {
	t.Parallel()

	const workers = 3
	const tasks = 20

	m := New(workers)
	w := NewWaiter()

	var inFlight, maxInFlight int64
	for i := 0; i < tasks; i++ {
		m.Run(func() error {
			n := atomic.AddInt64(&inFlight, 1)
			for {
				cur := atomic.LoadInt64(&maxInFlight)
				if n <= cur || atomic.CompareAndSwapInt64(&maxInFlight, cur, n) {
					break
				}
			}
			time.Sleep(5 * time.Millisecond)
			atomic.AddInt64(&inFlight, -1)
			return nil
		}, w)
	}

	if err := w.Wait(); err != nil {
		t.Fatal(err)
	}
	m.Close()

	if maxInFlight != workers {
		t.Fatalf("expected at most %d tasks in flight, and that many at peak; got %d", workers, maxInFlight)
	}
}

func TestWaiterCollectsEveryError(t *testing.T) {
	t.Parallel()

	m := New(4)

	var handled int64
	w := NewWaiter(WithErrorHandler(func(err error) {
		if err == nil {
			t.Error("handler received a nil error")
		}
		atomic.AddInt64(&handled, 1)
	}))

	want := 5
	for i := 0; i < 10; i++ {
		fail := i < want
		m.Run(func() error {
			if fail {
				return errors.New("task failed")
			}
			return nil
		}, w)
	}

	err := w.Wait()
	m.Close()

	if err == nil {
		t.Fatal("expected Wait to return the task errors")
	}
	var got int
	for _, e := range strings.Split(err.Error(), "\n") {
		if e == "task failed" {
			got++
		}
	}
	if got != want {
		t.Fatalf("expected %d errors joined, got %d: %v", want, got, err)
	}
	if handled != int64(want) {
		t.Fatalf("expected the handler to see %d errors, got %d", want, handled)
	}
}

// A failing task must never block: errors are recorded, not sent on a
// channel, so submitting more tasks than workers with no reader is fine.
func TestFailingTasksDoNotBlockSubmission(t *testing.T) {
	t.Parallel()

	m := New(2)
	w := NewWaiter()
	for i := 0; i < 20; i++ {
		m.Run(func() error { return errors.New("boom") }, w)
	}
	if err := w.Wait(); err == nil {
		t.Fatal("expected errors")
	}
	m.Close()
}

func TestWaitWithNoErrorsReturnsNil(t *testing.T) {
	t.Parallel()

	m := New(2)
	w := NewWaiter()
	m.Run(func() error { return nil }, w)
	if err := w.Wait(); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if err := w.Wait(); err != nil {
		t.Fatalf("second Wait: expected nil, got %v", err)
	}
	m.Close()
}

func TestCloseWaitsForRunningTasks(t *testing.T) {
	t.Parallel()

	m := New(2)
	w := NewWaiter()

	var finished int64
	for i := 0; i < 4; i++ {
		m.Run(func() error {
			time.Sleep(10 * time.Millisecond)
			atomic.AddInt64(&finished, 1)
			return nil
		}, w)
	}
	m.Close()
	if n := atomic.LoadInt64(&finished); n != 4 {
		t.Fatalf("Close returned with %d of 4 tasks finished", n)
	}
	if err := w.Wait(); err != nil {
		t.Fatal(err)
	}
}

func TestRunGuards(t *testing.T) {
	t.Parallel()

	mustPanic := func(name string, fn func()) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Errorf("%s: expected a panic", name)
			}
		}()
		fn()
	}

	m := New(2)
	w := NewWaiter()
	mustPanic("nil task", func() { m.Run(nil, w) })
	mustPanic("nil waiter", func() { m.Run(func() error { return nil }, nil) })

	m.Close()
	m.Close() // idempotent
	mustPanic("run after close", func() { m.Run(func() error { return nil }, w) })
}

func TestWaitIsIdempotent(t *testing.T) {
	t.Parallel()

	m := New(2)
	w := NewWaiter()
	m.Run(func() error { return nil }, w)
	if err := w.Wait(); err != nil {
		t.Fatal(err)
	}
	if err := w.Wait(); err != nil {
		t.Fatal(err)
	}
	m.Close()
}
