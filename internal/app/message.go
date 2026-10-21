package app

import (
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/akofink/akagent-cli/internal/store"
)

const (
	messageSendUsage   = "Usage: akagent message send --to <recipient> --from <sender> --text <message> --id <message-id> [--reply-to <message-id>] [--artifact <reference>] [--format <toon|json> (default toon)]"
	messageListenUsage = "Usage: akagent message listen --inbox <recipient> [--limit <1..100> (default 20)] [--wait <0..5m> (default immediate)] [--format <toon|json> (default toon)]"
	messageReadUsage   = "Usage: akagent message read <message-id> --inbox <recipient> [--format <toon|json> (default toon)]"
	messageAckUsage    = "Usage: akagent message ack <message-id> --inbox <recipient> --by <stable-address> [--format <toon|json> (default toon)]"
)

type messageView struct {
	ID        string `json:"id"`
	To        string `json:"to"`
	From      string `json:"from"`
	CreatedAt string `json:"created_at"`
	Text      string `json:"text"`
	ReplyTo   string `json:"reply_to,omitempty"`
	Artifact  string `json:"artifact,omitempty"`
	AckedAt   string `json:"acked_at,omitempty"`
	AckedBy   string `json:"acked_by,omitempty"`
}

type messageSendResult struct {
	Message messageView `json:"message"`
	Created bool        `json:"created"`
}

type messageDetailResult struct {
	Message messageView `json:"message"`
}

type messageListenResult struct {
	Inbox        string        `json:"inbox"`
	Messages     []messageView `json:"messages"`
	Count        int           `json:"count"`
	TotalPending int           `json:"total_pending"`
	HasMore      bool          `json:"has_more"`
	Empty        bool          `json:"empty"`
	TimedOut     bool          `json:"timed_out"`
}

type messageAckResult struct {
	Message messageView `json:"message"`
	Changed bool        `json:"changed"`
}

func messageCommand(args []string, stdout io.Writer) int {
	if len(args) == 1 && args[0] == "--help" {
		return write(stdout, helpView{Usage: "akagent message <send|listen|read|ack>", Commands: []string{"message send --help", "message listen --help", "message read --help", "message ack --help"}})
	}
	if len(args) == 0 {
		return messageUsageError(stdout)
	}
	switch args[0] {
	case "send":
		return messageSend(args[1:], stdout)
	case "listen":
		return messageListen(args[1:], stdout)
	case "read":
		return messageRead(args[1:], stdout)
	case "ack":
		return messageAck(args[1:], stdout)
	default:
		return messageUsageError(stdout)
	}
}

func messageUsageError(stdout io.Writer) int {
	return writeError(stdout, "usage", "Usage: akagent message <send|listen|read|ack>", false, "Run `akagent --help` for message commands")
}

func messageSend(args []string, stdout io.Writer) int {
	if len(args) == 1 && args[0] == "--help" {
		return write(stdout, helpView{Usage: messageSendUsage, Commands: []string{"akagent message send --to task:reviewer --from task:author --id stable-id --text \"Please review\"", "akagent message send --to inbox:ops --from agent:one --id stable-id --reply-to message-id --text \"Following up\""}})
	}
	values, format, ok := parseMessageFlags(args, map[string]bool{"--to": true, "--from": true, "--text": true, "--id": true, "--reply-to": true, "--artifact": true, "--format": true})
	if !ok || firstArg(args) != "" || values["--to"] == "" || values["--from"] == "" || values["--text"] == "" || values["--id"] == "" {
		return writeError(stdout, "usage", messageSendUsage, false, "Provide --to, --from, --text, and a retry-stable --id")
	}
	state, err := openMessageStore()
	if err != nil {
		return lifecycleError(stdout, err)
	}
	message := store.Message{ID: values["--id"], To: values["--to"], From: values["--from"], Text: values["--text"], ReplyTo: values["--reply-to"], Artifact: values["--artifact"], CreatedAt: time.Now().UTC()}
	created, result, err := state.CreateMessage(message)
	if err != nil {
		return lifecycleError(stdout, err)
	}
	return writeMessageFormat(stdout, messageSendResult{Message: presentMessage(result), Created: created}, format)
}

func messageListen(args []string, stdout io.Writer) int {
	if len(args) == 1 && args[0] == "--help" {
		return write(stdout, helpView{Usage: messageListenUsage, Commands: []string{"akagent message listen --inbox task:reviewer", "akagent message listen --inbox inbox:ops --limit 50 --wait 30s --format json"}})
	}
	values, format, ok := parseMessageFlags(args, map[string]bool{"--inbox": true, "--limit": true, "--wait": true, "--format": true})
	if !ok || firstArg(args) != "" || values["--inbox"] == "" {
		return writeError(stdout, "usage", messageListenUsage, false, "Provide --inbox and optional bounded --limit/--wait")
	}
	limit := 20
	if raw := values["--limit"]; raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 100 {
			return writeError(stdout, "usage", messageListenUsage, false, "Use --limit with a value from 1 through 100")
		}
		limit = parsed
	}
	wait := time.Duration(0)
	hasWait := values["--wait"] != ""
	if hasWait {
		parsed, err := time.ParseDuration(values["--wait"])
		if err != nil || parsed < 0 || parsed > 5*time.Minute {
			return writeError(stdout, "usage", messageListenUsage, false, "Use a finite --wait duration from 0 through 5m")
		}
		wait = parsed
	}
	state, err := openMessageStore()
	if err != nil {
		return lifecycleError(stdout, err)
	}
	deadline := time.Now().Add(wait)
	var messages []store.Message
	totalPending := 0
	for {
		messages, totalPending, err = state.PendingMessageBatch(values["--inbox"], limit)
		if err != nil {
			return lifecycleError(stdout, err)
		}
		if len(messages) != 0 || !hasWait || !time.Now().Before(deadline) {
			break
		}
		time.Sleep(min(100*time.Millisecond, time.Until(deadline)))
	}
	views := make([]messageView, 0, len(messages))
	for _, message := range messages {
		views = append(views, presentMessage(message))
	}
	return writeMessageFormat(stdout, messageListenResult{Inbox: values["--inbox"], Messages: views, Count: len(views), TotalPending: totalPending, HasMore: totalPending > len(views), Empty: len(views) == 0, TimedOut: hasWait && len(views) == 0}, format)
}

func messageRead(args []string, stdout io.Writer) int {
	if len(args) == 1 && args[0] == "--help" {
		return write(stdout, helpView{Usage: messageReadUsage, Commands: []string{"akagent message read <message-id> --inbox task:reviewer", "akagent message read <message-id> --inbox inbox:ops --format json"}})
	}
	id, rest := popFirstArg(args)
	values, format, ok := parseMessageFlags(rest, map[string]bool{"--inbox": true, "--format": true})
	if !ok || id == "" || values["--inbox"] == "" {
		return writeError(stdout, "usage", messageReadUsage, false, "Provide a message ID and its --inbox address")
	}
	state, err := openMessageStore()
	if err != nil {
		return lifecycleError(stdout, err)
	}
	message, err := state.ReadMessage(values["--inbox"], id)
	if err != nil {
		return lifecycleError(stdout, err)
	}
	return writeMessageFormat(stdout, messageDetailResult{Message: presentMessage(message)}, format)
}

func messageAck(args []string, stdout io.Writer) int {
	if len(args) == 1 && args[0] == "--help" {
		return write(stdout, helpView{Usage: messageAckUsage, Commands: []string{"akagent message ack <message-id> --inbox task:reviewer --by task:agent", "akagent message ack <message-id> --inbox inbox:ops --by worker:one --format json"}})
	}
	id, rest := popFirstArg(args)
	values, format, ok := parseMessageFlags(rest, map[string]bool{"--inbox": true, "--by": true, "--format": true})
	if !ok || id == "" || values["--inbox"] == "" || values["--by"] == "" {
		return writeError(stdout, "usage", messageAckUsage, false, "Provide a message ID, --inbox, and --by stable address")
	}
	state, err := openMessageStore()
	if err != nil {
		return lifecycleError(stdout, err)
	}
	message, changed, err := state.AcknowledgeMessage(values["--inbox"], id, values["--by"], time.Now().UTC())
	if err != nil {
		return lifecycleError(stdout, err)
	}
	return writeMessageFormat(stdout, messageAckResult{Message: presentMessage(message), Changed: changed}, format)
}

func parseMessageFlags(args []string, known map[string]bool) (map[string]string, string, bool) {
	values := make(map[string]string)
	format := "toon"
	for index := 0; index < len(args); {
		flag := args[index]
		if !known[flag] || values[flag] != "" || index+1 >= len(args) || args[index+1] == "" {
			return nil, "", false
		}
		value := args[index+1]
		if flag == "--format" {
			if value != "toon" && value != "json" {
				return nil, "", false
			}
			format = value
		}
		values[flag] = value
		index += 2
	}
	return values, format, true
}

func firstArg(args []string) string {
	if len(args) == 0 || strings.HasPrefix(args[0], "--") {
		return ""
	}
	return args[0]
}

func popFirstArg(args []string) (string, []string) {
	if len(args) == 0 || strings.HasPrefix(args[0], "--") {
		return "", args
	}
	return args[0], args[1:]
}

func presentMessage(message store.Message) messageView {
	view := messageView{ID: message.ID, To: message.To, From: message.From, CreatedAt: message.CreatedAt.UTC().Format(time.RFC3339Nano), Text: message.Text, ReplyTo: message.ReplyTo, Artifact: message.Artifact, AckedBy: message.AckedBy}
	if message.AckedAt != nil {
		view.AckedAt = message.AckedAt.UTC().Format(time.RFC3339Nano)
	}
	return view
}

func openMessageStore() (*store.Store, error) {
	return store.Open()
}

func writeMessageFormat(stdout io.Writer, value any, format string) int {
	if format == "json" {
		return writeJSON(stdout, value)
	}
	return write(stdout, value)
}
