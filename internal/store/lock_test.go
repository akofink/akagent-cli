package store

import (
	"errors"
	"strings"
	"testing"
)

// TestLockContentionIsRetryable verifies a contended per-task lock returns a
// typed retryable error and that the lock is usable again after release.
func TestLockContentionIsRetryable(t *testing.T) {
	store := openTest(t)
	taskID := validTaskID(t)
	withShortLockWait(t, func() {
		release, err := store.Lock(taskID)
		if err != nil {
			t.Fatalf("Lock() error = %v", err)
		}

		_, contenderErr := store.Lock(taskID)
		if !IsKind(contenderErr, KindLocked) {
			t.Fatalf("contending Lock() error = %v, want KindLocked", contenderErr)
		}
		if !isRetryable(contenderErr) {
			t.Fatalf("contending Lock() error %v is not retryable", contenderErr)
		}

		if err := release(); err != nil {
			t.Fatalf("release() error = %v", err)
		}
		if err := release(); err != nil {
			t.Fatalf("second release() error = %v, want idempotent", err)
		}

		releaseTwo, err := store.Lock(taskID)
		if err != nil {
			t.Fatalf("Lock() after release error = %v", err)
		}
		if err := releaseTwo(); err != nil {
			t.Fatalf("releaseTwo() error = %v", err)
		}
	})
}

// TestWithLockRunsUnderLock verifies WithLock serializes a callback and
// surfaces the callback error.
func TestWithLockRunsUnderLock(t *testing.T) {
	store := openTest(t)
	taskID := validTaskID(t)
	withShortLockWait(t, func() {
		err := store.WithLock(taskID, func() error {
			_, nestedErr := store.Lock(taskID)
			if !IsKind(nestedErr, KindLocked) {
				return &Error{Kind: KindInternal, Message: "nested lock unexpectedly acquired"}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("WithLock() error = %v", err)
		}
	})
}

func TestWithLockReturnsCallbackError(t *testing.T) {
	store := openTest(t)
	taskID := validTaskID(t)
	wanted := &Error{Kind: KindMalformed, Message: "callback error"}
	err := store.WithLock(taskID, func() error { return wanted })
	if !errors.Is(err, wanted) {
		t.Fatalf("WithLock() error = %v, want callback error %v", err, wanted)
	}

	// Verify the lock was released by re-acquiring it.
	release, err := store.Lock(taskID)
	if err != nil {
		t.Fatalf("Lock() after WithLock error = %v (lock not released)", err)
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
}

func TestWithLockPreservesCallbackAndUnlockFailures(t *testing.T) {
	store := openTest(t)
	taskID := validTaskID(t)
	callbackErr := errors.New("callback failed")
	unlockErr := errors.New("unlock failed")
	store.unlockFn = func() error { return unlockErr }
	err := store.WithLock(taskID, func() error { return callbackErr })
	if !IsKind(err, KindPartial) {
		t.Fatalf("WithLock() error = %v, want KindPartial", err)
	}
	if !errors.Is(err, callbackErr) || !errors.Is(err, unlockErr) {
		t.Fatalf("WithLock() error = %v, want both callback and unlock errors", err)
	}
}

func TestWithLockSurfacesUnlockFailure(t *testing.T) {
	store := openTest(t)
	taskID := validTaskID(t)
	store.unlockFn = func() error { return errors.New("unlock failed") }
	err := store.WithLock(taskID, func() error { return nil })
	if !IsKind(err, KindInternal) {
		t.Fatalf("WithLock() error = %v, want KindInternal", err)
	}
	if !strings.Contains(err.Error(), "lock") {
		t.Fatalf("WithLock() error = %q, want lock-release hint", err.Error())
	}
}

func isRetryable(err error) bool {
	var storeErr *Error
	if !errors.As(err, &storeErr) {
		return false
	}
	return storeErr.Retryable
}
