package store

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestCreateMessageRetryConflictAndRestart(t *testing.T) {
	state := openTest(t)
	message := testMessage("retry-id", "task:one", "sender:one")
	created, got, err := state.CreateMessage(message)
	if err != nil || !created || got.ID != message.ID {
		t.Fatalf("CreateMessage() = (%v, %#v, %v), want created message", created, got, err)
	}
	created, got, err = state.CreateMessage(message)
	if err != nil || created || got.ID != message.ID {
		t.Fatalf("idempotent CreateMessage() = (%v, %#v, %v), want existing message", created, got, err)
	}
	message.Text = "different body"
	if _, _, err := state.CreateMessage(message); !IsKind(err, KindConflict) {
		t.Fatalf("conflicting CreateMessage() error = %v, want conflict", err)
	}

	reopened, err := OpenAt(state.Root())
	if err != nil {
		t.Fatal(err)
	}
	got, err = reopened.ReadMessage("task:one", "retry-id")
	if err != nil || got.Text != "hello" {
		t.Fatalf("ReadMessage() after reopen = (%#v, %v), want persisted message", got, err)
	}
}

func TestConcurrentIdempotentRetriesCreateOneMessage(t *testing.T) {
	state := openTest(t)
	message := testMessage("same-retry-id", "inbox:retry", "sender:retry")
	const attempts = 12
	var wait sync.WaitGroup
	created := make(chan bool, attempts)
	errs := make(chan error, attempts)
	for range attempts {
		wait.Add(1)
		go func() {
			defer wait.Done()
			wasCreated, _, err := state.CreateMessage(message)
			created <- wasCreated
			errs <- err
		}()
	}
	wait.Wait()
	close(created)
	close(errs)
	createdCount := 0
	for wasCreated := range created {
		if wasCreated {
			createdCount++
		}
	}
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent retry error = %v", err)
		}
	}
	if createdCount != 1 {
		t.Fatalf("successful create count = %d, want 1", createdCount)
	}
	messages, err := state.PendingMessages("inbox:retry", 100)
	if err != nil || len(messages) != 1 {
		t.Fatalf("PendingMessages() = (%d, %v), want one idempotent record", len(messages), err)
	}
}

func TestMessageConcurrentSendAndAck(t *testing.T) {
	state := openTest(t)
	const count = 16
	var wait sync.WaitGroup
	errs := make(chan error, count)
	for i := 0; i < count; i++ {
		wait.Add(1)
		go func(i int) {
			defer wait.Done()
			_, _, err := state.CreateMessage(testMessage(fmt.Sprintf("message-%02d", i), "task:concurrent", "sender:concurrent"))
			errs <- err
		}(i)
	}
	wait.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent CreateMessage() error = %v", err)
		}
	}

	messages, total, err := state.PendingMessageBatch("task:concurrent", 10)
	if err != nil || len(messages) != 10 || total != count {
		t.Fatalf("PendingMessageBatch() = (%d messages, total %d, %v), want 10 of %d", len(messages), total, err, count)
	}
	messages, err = state.PendingMessages("task:concurrent", 100)
	if err != nil || len(messages) != count {
		t.Fatalf("PendingMessages() = (%d, %v), want %d", len(messages), err, count)
	}
	var ackWait sync.WaitGroup
	ackErrs := make(chan error, count)
	for _, message := range messages {
		ackWait.Add(1)
		go func(id string) {
			defer ackWait.Done()
			_, _, err := state.AcknowledgeMessage("task:concurrent", id, "worker:one", time.Now())
			ackErrs <- err
		}(message.ID)
	}
	ackWait.Wait()
	close(ackErrs)
	for err := range ackErrs {
		if err != nil {
			t.Fatalf("concurrent AcknowledgeMessage() error = %v", err)
		}
	}
	pending, err := state.PendingMessages("task:concurrent", 100)
	if err != nil || len(pending) != 0 {
		t.Fatalf("PendingMessages() after concurrent ack = (%d, %v), want empty", len(pending), err)
	}
}

func TestPendingMessagesReadDoesNotAcknowledgeAndAckIsIdempotent(t *testing.T) {
	state := openTest(t)
	message := testMessage("pending", "inbox:agent", "agent:sender")
	if _, _, err := state.CreateMessage(message); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		pending, err := state.PendingMessages("inbox:agent", 20)
		if err != nil || len(pending) != 1 {
			t.Fatalf("PendingMessages() = (%d, %v), want the still-pending message", len(pending), err)
		}
	}
	ack, changed, err := state.AcknowledgeMessage("inbox:agent", message.ID, "agent:reader", time.Now())
	if err != nil || !changed || ack.AckedAt == nil {
		t.Fatalf("AcknowledgeMessage() = (%#v, %v, %v), want recorded ack", ack, changed, err)
	}
	originalAck := *ack.AckedAt
	ack, changed, err = state.AcknowledgeMessage("inbox:agent", message.ID, "agent:other", time.Now().Add(time.Second))
	if err != nil || changed || ack.AckedBy != "agent:reader" || !ack.AckedAt.Equal(originalAck) {
		t.Fatalf("repeated AcknowledgeMessage() = (%#v, %v, %v), want original ack", ack, changed, err)
	}
}

func TestMessageValidationAndInboxIsolation(t *testing.T) {
	state := openTest(t)
	for _, message := range []Message{
		{ID: "unsafe/id", To: "inbox", From: "sender", Text: "hello"},
		{ID: "valid", To: "inbox", From: "sender", Text: "  "},
		{ID: "long", To: "inbox", From: "sender", Text: string(make([]byte, maxMessageTextBytes+1))},
		{ID: "unsafe-ack", To: "inbox", From: "sender", Text: "hello", AckedBy: "spoof"},
	} {
		if _, _, err := state.CreateMessage(message); !IsKind(err, KindUsage) {
			t.Errorf("CreateMessage(%q) error = %v, want usage", message.ID, err)
		}
	}
	message := testMessage("private-to-inbox", "inbox:one", "sender")
	if _, _, err := state.CreateMessage(message); err != nil {
		t.Fatal(err)
	}
	if _, err := state.ReadMessage("inbox:two", message.ID); !IsKind(err, KindNotFound) {
		t.Fatalf("cross-inbox ReadMessage() error = %v, want not found", err)
	}
	if _, err := state.PendingMessages("inbox:one", 101); !IsKind(err, KindUsage) {
		t.Fatalf("oversized batch error = %v, want usage", err)
	}
}

func testMessage(id, to, from string) Message {
	return Message{ID: id, To: to, From: from, Text: "hello", CreatedAt: time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)}
}
