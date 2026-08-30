package store

import (
	"fmt"
	"sync"
	"testing"
)

// retryableWrite runs fn, retrying when the per-task lock is contended.
// Lock contention is intentionally a retryable error, so stress tests retry
// it and only surface genuine failures.
func retryOnContention(t *testing.T, fn func() (int, error)) (int, error) {
	t.Helper()
	const attempts = 50
	for attempt := 0; attempt < attempts; attempt++ {
		value, err := fn()
		if err == nil {
			return value, nil
		}
		if !IsKind(err, KindLocked) {
			return value, err
		}
	}
	return 0, fmt.Errorf("operation still contended after %d attempts", attempts)
}

// TestConcurrentLockCreation verifies concurrent first use of a task lock
// does not lose the lock file creation race.
func TestConcurrentLockCreation(t *testing.T) {
	store := openTest(t)
	taskID := validTaskID(t)
	withConcurrencyLockWait(t, func() {
		const contenders = 8
		var wg sync.WaitGroup
		errs := make(chan error, contenders)
		start := make(chan struct{})
		for i := 0; i < contenders; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				release, err := store.Lock(taskID)
				if err != nil {
					errs <- err
					return
				}
				if err := release(); err != nil {
					errs <- err
				}
			}()
		}
		close(start)
		wg.Wait()
		close(errs)
		for err := range errs {
			t.Errorf("Lock() error = %v", err)
		}
	})
}

// TestConcurrentWritesSerialize runs many writer goroutines that each fully
// replace the manifest under the per-task lock. The final manifest must be
// exactly one writer's complete value, never a mixture.
func TestConcurrentWritesSerialize(t *testing.T) {
	store := openTest(t)
	taskID := validTaskID(t)
	withConcurrencyLockWait(t, func() {
		const writers = 8
		const iterations = 20
		var wg sync.WaitGroup
		errs := make(chan error, writers)
		for w := 0; w < writers; w++ {
			wg.Add(1)
			go func(w int) {
				defer wg.Done()
				for i := 0; i < iterations; i++ {
					title := fmt.Sprintf("writer-%d-%d", w, i)
					if _, err := retryOnContention(t, func() (int, error) {
						return 0, store.WriteManifest(taskID, Manifest{Title: title, Worker: "local", Lifecycle: "running"})
					}); err != nil {
						errs <- err
						return
					}
				}
			}(w)
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			t.Errorf("WriteManifest() error = %v", err)
		}

		envelope, err := store.ReadManifest(taskID)
		if err != nil {
			t.Fatalf("ReadManifest() error = %v", err)
		}
		got, err := envelope.DecodeManifest()
		if err != nil {
			t.Fatalf("DecodeManifest() error = %v", err)
		}
		if !matchesConcurrentTitle(got.Title) {
			t.Fatalf("final manifest title = %q, want one complete writer value", got.Title)
		}
	})
}

// TestConcurrentReadsRejectPartialWrites keeps readers reading while writers
// replace the manifest. Atomic replacement guarantees readers never observe a
// truncated or malformed record.
func TestConcurrentReadsRejectPartialWrites(t *testing.T) {
	store := openTest(t)
	taskID := validTaskID(t)
	if err := store.WriteManifest(taskID, Manifest{Title: "initial", Lifecycle: "starting"}); err != nil {
		t.Fatal(err)
	}

	withConcurrencyLockWait(t, func() {
		fill := func(n int) string {
			payload := make([]byte, n)
			for i := range payload {
				payload[i] = 'a' + byte(i%26)
			}
			return string(payload)
		}
		const readerCount = 4
		const writerCount = 6
		done := make(chan struct{})
		readErrs := make(chan error, readerCount)
		writerErrs := make(chan error, writerCount)

		var readers sync.WaitGroup
		readers.Add(readerCount)
		for r := 0; r < readerCount; r++ {
			go func() {
				defer readers.Done()
				for {
					select {
					case <-done:
						return
					default:
					}
					envelope, err := store.ReadManifest(taskID)
					if err != nil {
						readErrs <- fmt.Errorf("ReadManifest() error = %v", err)
						return
					}
					if _, err := envelope.DecodeManifest(); err != nil {
						readErrs <- fmt.Errorf("DecodeManifest() error = %v (partial read)", err)
						return
					}
				}
			}()
		}

		var writers sync.WaitGroup
		writers.Add(writerCount)
		for w := 0; w < writerCount; w++ {
			go func(w int) {
				defer writers.Done()
				for i := 0; i < 30; i++ {
					title := fmt.Sprintf("%d-%d-%s", w, i, fill(4096))
					if _, err := retryOnContention(t, func() (int, error) {
						return 0, store.WriteManifest(taskID, Manifest{Title: title, Lifecycle: "running"})
					}); err != nil {
						writerErrs <- err
						return
					}
				}
			}(w)
		}
		writers.Wait()
		close(done)
		readers.Wait()
		close(readErrs)
		close(writerErrs)
		for err := range writerErrs {
			t.Errorf("writer error = %v", err)
		}
		for err := range readErrs {
			t.Errorf("reader error = %v", err)
		}
	})
}

// TestConcurrentAppendEventsSerialized verifies appended events receive a
// contiguous, non-overlapping sequence under concurrency.
func TestConcurrentAppendEventsSerialized(t *testing.T) {
	store := openTest(t)
	taskID := validTaskID(t)
	withConcurrencyLockWait(t, func() {
		const appenders = 5
		const perAppender = 20
		total := appenders * perAppender
		var wg sync.WaitGroup
		seqErrs := make(chan error, appenders)
		seen := make([]bool, total+1)
		var mutex sync.Mutex
		for a := 0; a < appenders; a++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < perAppender; i++ {
					sequence, err := retryOnContention(t, func() (int, error) {
						return store.AppendEvent(taskID, Event{Operation: "publish", Outcome: "ok"})
					})
					if err != nil {
						seqErrs <- err
						return
					}
					mutex.Lock()
					if sequence < 1 || sequence > total || seen[sequence] {
						mutex.Unlock()
						seqErrs <- fmt.Errorf("AppendEvent() duplicate or out-of-range sequence %d", sequence)
						return
					}
					seen[sequence] = true
					mutex.Unlock()
				}
			}()
		}
		wg.Wait()
		close(seqErrs)
		for err := range seqErrs {
			t.Errorf("append error = %v", err)
		}

		for i := 1; i <= total; i++ {
			if !seen[i] {
				t.Errorf("sequence %d missing, want contiguous events", i)
			}
		}
		records, err := store.ReadEvents(taskID)
		if err != nil {
			t.Fatalf("ReadEvents() error = %v", err)
		}
		if len(records) != total {
			t.Fatalf("ReadEvents() len = %d, want %d", len(records), total)
		}
	})
}

func matchesConcurrentTitle(title string) bool {
	for _, w := range []int{0, 1, 2, 3, 4, 5, 6, 7} {
		for i := 0; i < 20; i++ {
			if title == fmt.Sprintf("writer-%d-%d", w, i) {
				return true
			}
		}
	}
	return false
}
