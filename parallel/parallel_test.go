package parallel

import (
	"errors"
	"runtime"
	"sync"
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

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range w.Err() {
		}
	}()
	w.Wait()
	<-done
	m.Close()

	if maxInFlight != workers {
		t.Fatalf("expected at most %d tasks in flight, and that many at peak; got %d", workers, maxInFlight)
	}
}

func TestWaiterDeliversEveryError(t *testing.T) {
	t.Parallel()

	m := New(4)
	w := NewWaiter()

	// Drain Err before submitting: a failing task blocks on the unbuffered
	// error channel, and with the pool full that would block Run itself.
	var got int
	var mu sync.Mutex
	done := make(chan struct{})
	go func() {
		defer close(done)
		for err := range w.Err() {
			mu.Lock()
			got++
			mu.Unlock()
			if err == nil {
				t.Error("received a nil error")
			}
		}
	}()

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

	w.Wait() // closes Err() once every task has finished
	<-done
	m.Close()

	if got != want {
		t.Fatalf("expected %d errors, got %d", want, got)
	}
}

// Err is unbuffered: a task that returns an error blocks until someone reads
// it, so the error channel must be drained concurrently with Wait. This test
// documents that contract; a caller that only calls Wait would deadlock.
func TestWaiterErrIsUnbuffered(t *testing.T) {
	t.Parallel()

	m := New(2)
	w := NewWaiter()

	finished := make(chan struct{})
	m.Run(func() error { return errors.New("blocked until read") }, w)
	go func() {
		w.Wait()
		close(finished)
	}()

	select {
	case <-finished:
		t.Fatal("Wait returned before the error was read")
	case <-time.After(50 * time.Millisecond):
	}

	if err := <-w.Err(); err == nil {
		t.Fatal("expected the task's error")
	}
	<-finished
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
	go func() {
		for range w.Err() {
		}
	}()

	m.Close()
	if n := atomic.LoadInt64(&finished); n != 4 {
		t.Fatalf("Close returned with %d of 4 tasks finished", n)
	}
	w.Wait()
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
	w.Wait()
	w.Wait() // must not panic with "close of closed channel"
	if _, open := <-w.Err(); open {
		t.Fatal("expected Err to be closed after Wait")
	}
	m.Close()
}
