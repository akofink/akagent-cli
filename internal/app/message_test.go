package app

import (
	"os"
	"strings"
	"testing"
)

func TestMessageRoundTripAndRestart(t *testing.T) {
	setupTaskCommandTest(t)
	sent := runCommand(t, []string{"message", "send", "--to", "task:recipient", "--from", "task:sender", "--text", "please review", "--id", "message-1", "--reply-to", "prior-1", "--artifact", "file:review.diff", "--format", "json"})
	if sent.code != 0 || !strings.Contains(sent.stdout, `"id":"message-1"`) || !strings.Contains(sent.stdout, `"reply_to":"prior-1"`) || !strings.Contains(sent.stdout, `"created":true`) {
		t.Fatalf("send = (%d, %q)", sent.code, sent.stdout)
	}
	retry := runCommand(t, []string{"message", "send", "--to", "task:recipient", "--from", "task:sender", "--text", "please review", "--id", "message-1", "--reply-to", "prior-1", "--artifact", "file:review.diff", "--format", "json"})
	if retry.code != 0 || !strings.Contains(retry.stdout, `"created":false`) {
		t.Fatalf("retry = (%d, %q), want idempotent no-op", retry.code, retry.stdout)
	}
	list := runCommand(t, []string{"message", "listen", "--inbox", "task:recipient", "--limit", "10", "--format", "json"})
	if list.code != 0 || !strings.Contains(list.stdout, `"count":1`) || !strings.Contains(list.stdout, `"empty":false`) || !strings.Contains(list.stdout, "please review") {
		t.Fatalf("listen = (%d, %q)", list.code, list.stdout)
	}
	read := runCommand(t, []string{"message", "read", "message-1", "--inbox", "task:recipient", "--format", "json"})
	if read.code != 0 || !strings.Contains(read.stdout, "please review") {
		t.Fatalf("read after separate CLI invocation = (%d, %q)", read.code, read.stdout)
	}
	ack := runCommand(t, []string{"message", "ack", "message-1", "--inbox", "task:recipient", "--by", "task:worker", "--format", "json"})
	if ack.code != 0 || !strings.Contains(ack.stdout, `"changed":true`) || !strings.Contains(ack.stdout, `"acked_by":"task:worker"`) {
		t.Fatalf("ack = (%d, %q)", ack.code, ack.stdout)
	}
	retryAck := runCommand(t, []string{"message", "ack", "message-1", "--inbox", "task:recipient", "--by", "task:other", "--format", "json"})
	if retryAck.code != 0 || !strings.Contains(retryAck.stdout, `"changed":false`) || !strings.Contains(retryAck.stdout, `"acked_by":"task:worker"`) {
		t.Fatalf("ack retry = (%d, %q)", retryAck.code, retryAck.stdout)
	}
	empty := runCommand(t, []string{"message", "listen", "--inbox", "task:recipient", "--format", "json"})
	if empty.code != 0 || !strings.Contains(empty.stdout, `"count":0`) || !strings.Contains(empty.stdout, `"empty":true`) {
		t.Fatalf("empty listen = (%d, %q)", empty.code, empty.stdout)
	}
}

func TestMessageDefaultOutputAndHelp(t *testing.T) {
	setupTaskCommandTest(t)
	sent := runCommand(t, []string{"message", "send", "--to", "task:reader", "--from", "task:writer", "--text", "hello", "--id", "toon-output"})
	if sent.code != 0 || !strings.Contains(sent.stdout, "message:") || !strings.Contains(sent.stdout, "created: true") {
		t.Fatalf("default send output = (%d, %q), want TOON", sent.code, sent.stdout)
	}
	help := runCommand(t, []string{"message", "listen", "--help"})
	if help.code != 0 || !strings.Contains(help.stdout, "--limit") || !strings.Contains(help.stdout, "--wait") || !strings.Contains(help.stdout, "--format") {
		t.Fatalf("listen help = (%d, %q), want complete options", help.code, help.stdout)
	}
}

func TestMessageListenFiniteTimeout(t *testing.T) {
	setupTaskCommandTest(t)
	result := runCommand(t, []string{"message", "listen", "--inbox", "task:quiet", "--wait", "20ms", "--format", "json"})
	if result.code != 0 || !strings.Contains(result.stdout, `"empty":true`) || !strings.Contains(result.stdout, `"timed_out":true`) {
		t.Fatalf("finite empty listen = (%d, %q)", result.code, result.stdout)
	}
}

func TestMessageRejectsInvalidInputs(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_STATE_HOME", root+"/state")
	t.Setenv("HOME", root+"/home")
	for _, args := range [][]string{
		{"message", "send", "--to", "inbox", "--from", "sender", "--text", "hello"},
		{"message", "send", "--to", "inbox", "--from", "sender", "--text", "hello", "--id", "x", "--unknown", "bad"},
		{"message", "listen", "--inbox", "inbox", "--limit", "101"},
		{"message", "listen", "--inbox", "inbox", "--wait", "6m"},
		{"message", "listen", "--inbox", "inbox", "--format", "yaml"},
		{"message", "read", "missing", "--inbox", "other", "--extra", "x"},
		{"message", "ack", "missing", "--inbox", "inbox", "--by", "worker", "--extra", "x"},
	} {
		result := runCommand(t, args)
		if result.code != 2 || !strings.Contains(result.stdout, "error:") || !strings.Contains(result.stdout, "category: usage") {
			t.Errorf("Run(%q) = (%d, %q), want structured usage error", args, result.code, result.stdout)
		}
	}
	if _, err := os.Stat(root + "/state/akagent"); !os.IsNotExist(err) {
		t.Fatalf("invalid commands opened store, stat error = %v", err)
	}
}

func TestMessageInboxIsolationAndNotFound(t *testing.T) {
	setupTaskCommandTest(t)
	if result := runCommand(t, []string{"message", "send", "--to", "inbox:a", "--from", "sender", "--text", "secret", "--id", "isolation"}); result.code != 0 {
		t.Fatalf("send = (%d, %q)", result.code, result.stdout)
	}
	read := runCommand(t, []string{"message", "read", "isolation", "--inbox", "inbox:b"})
	if read.code != 1 || !strings.Contains(read.stdout, "not available in this inbox") {
		t.Fatalf("cross-inbox read = (%d, %q), want not found", read.code, read.stdout)
	}
}
