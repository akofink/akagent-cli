package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	messageSchemaVersion = 1
	messageLockID        = "messages"
	maxMessageIDBytes    = 128
	maxMessageAddress    = 128
	maxMessageTextBytes  = 16 * 1024
	maxMessageArtifact   = 2048
)

var messageAddressPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:@+-]{0,127}$`)

// Message is a bounded, caller-authored communication record. Sender and
// recipient values are metadata only; they confer no lifecycle authority.
type Message struct {
	ID        string     `json:"id"`
	To        string     `json:"to"`
	From      string     `json:"from"`
	CreatedAt time.Time  `json:"created_at"`
	Text      string     `json:"text"`
	ReplyTo   string     `json:"reply_to,omitempty"`
	Artifact  string     `json:"artifact,omitempty"`
	AckedAt   *time.Time `json:"acked_at,omitempty"`
	AckedBy   string     `json:"acked_by,omitempty"`
}

type messageEnvelope struct {
	SchemaVersion int       `json:"schema_version"`
	Kind          string    `json:"kind"`
	ObservedAt    time.Time `json:"observed_at"`
	Message       Message   `json:"message"`
}

// CreateMessage persists a message once by globally unique ID. An equivalent
// retry returns created=false; reusing the ID with different content conflicts.
func (s *Store) CreateMessage(message Message) (created bool, existing Message, err error) {
	if err := validateMessage(message); err != nil {
		return false, Message{}, err
	}
	if message.CreatedAt.IsZero() {
		message.CreatedAt = time.Now().UTC()
	} else {
		message.CreatedAt = message.CreatedAt.UTC()
	}
	err = s.WithLock(messageLockID, func() error {
		path := s.messagePath(message.ID)
		data, readErr := s.readOwnedFile(path)
		if readErr == nil {
			existing, readErr = decodeMessage(path, data)
			if readErr != nil {
				return readErr
			}
			if sameMessageInput(existing, message) {
				return nil
			}
			return newError(KindConflict, "Message ID is already used with different content", "Send again with a new message ID")
		}
		if !IsKind(readErr, KindNotFound) {
			return readErr
		}
		if err := s.ensureDir(s.messagesDir(), "messages directory"); err != nil {
			return err
		}
		envelope := messageEnvelope{SchemaVersion: messageSchemaVersion, Kind: "message", ObservedAt: message.CreatedAt, Message: message}
		encoded, marshalErr := json.MarshalIndent(envelope, "", "  ")
		if marshalErr != nil {
			return internalError("encode a message", "Retry the send operation")
		}
		if err := s.atomicallyWrite(path, append(encoded, '\n')); err != nil {
			return err
		}
		created = true
		existing = message
		return nil
	})
	return created, existing, err
}

// ReadMessage returns a message only when it belongs to the addressed inbox.
func (s *Store) ReadMessage(inbox, id string) (Message, error) {
	if err := validateMessageAddress("inbox", inbox); err != nil {
		return Message{}, err
	}
	if err := validateMessageID(id); err != nil {
		return Message{}, err
	}
	var message Message
	err := s.WithLock(messageLockID, func() error {
		data, err := s.readOwnedFile(s.messagePath(id))
		if err != nil {
			return err
		}
		message, err = decodeMessage(s.messagePath(id), data)
		if err != nil {
			return err
		}
		if message.To != inbox {
			return newError(KindNotFound, "Message is not available in this inbox", "Check the recipient address and message ID")
		}
		return nil
	})
	return message, err
}

// PendingMessages returns the oldest unacknowledged messages for inbox, in
// deterministic creation-time and ID order. Reading never acknowledges.
func (s *Store) PendingMessages(inbox string, limit int) ([]Message, error) {
	messages, _, err := s.PendingMessageBatch(inbox, limit)
	return messages, err
}

// PendingMessageBatch returns at most limit pending messages and the total
// number pending for the addressed inbox.
func (s *Store) PendingMessageBatch(inbox string, limit int) ([]Message, int, error) {
	if err := validateMessageAddress("inbox", inbox); err != nil {
		return nil, 0, err
	}
	if limit < 1 || limit > 100 {
		return nil, 0, newError(KindUsage, "Message batch limit must be between 1 and 100", "Use --limit with a value from 1 through 100")
	}
	messages, err := s.listMessages()
	if err != nil {
		return nil, 0, err
	}
	pending := make([]Message, 0, limit)
	total := 0
	for _, message := range messages {
		if message.To == inbox && message.AckedAt == nil {
			total++
			if len(pending) < limit {
				pending = append(pending, message)
			}
		}
	}
	return pending, total, nil
}

// AcknowledgeMessage records the first explicit acknowledgement. Equivalent
// retries are idempotent and preserve the original ack identity and time.
func (s *Store) AcknowledgeMessage(inbox, id, by string, at time.Time) (Message, bool, error) {
	if err := validateMessageAddress("inbox", inbox); err != nil {
		return Message{}, false, err
	}
	if err := validateMessageID(id); err != nil {
		return Message{}, false, err
	}
	if err := validateMessageAddress("acknowledger", by); err != nil {
		return Message{}, false, err
	}
	var message Message
	changed := false
	err := s.WithLock(messageLockID, func() error {
		path := s.messagePath(id)
		data, err := s.readOwnedFile(path)
		if err != nil {
			return err
		}
		message, err = decodeMessage(path, data)
		if err != nil {
			return err
		}
		if message.To != inbox {
			return newError(KindNotFound, "Message is not available in this inbox", "Check the recipient address and message ID")
		}
		if message.AckedAt != nil {
			return nil
		}
		if at.IsZero() {
			at = time.Now().UTC()
		} else {
			at = at.UTC()
		}
		message.AckedAt = &at
		message.AckedBy = by
		encoded, marshalErr := json.MarshalIndent(messageEnvelope{SchemaVersion: messageSchemaVersion, Kind: "message", ObservedAt: message.CreatedAt, Message: message}, "", "  ")
		if marshalErr != nil {
			return internalError("encode message acknowledgement", "Retry the acknowledgement")
		}
		if err := s.atomicallyWrite(path, append(encoded, '\n')); err != nil {
			return err
		}
		changed = true
		return nil
	})
	return message, changed, err
}

func (s *Store) listMessages() ([]Message, error) {
	var messages []Message
	err := s.WithLock(messageLockID, func() error {
		dir, err := s.openOwned(s.messagesDir(), true)
		if IsKind(err, KindNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		entries, err := dir.ReadDir(-1)
		closeErr := dir.Close()
		if err != nil {
			return internalError("list message records", "Check the message store and retry")
		}
		if closeErr != nil {
			return internalError("close the message directory", "Retry the listen operation")
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), tempPrefix) {
				continue
			}
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				return malformedError("Message store contains an unexpected entry", "Inspect and repair the message store")
			}
			path := filepath.Join(s.messagesDir(), entry.Name())
			data, err := s.readOwnedFile(path)
			if err != nil {
				return err
			}
			message, err := decodeMessage(path, data)
			if err != nil {
				return err
			}
			if s.messagePath(message.ID) != path {
				return malformedError("Message filename does not match its ID", "Inspect and repair the message store")
			}
			messages = append(messages, message)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(messages, func(i, j int) bool {
		if messages[i].CreatedAt.Equal(messages[j].CreatedAt) {
			return messages[i].ID < messages[j].ID
		}
		return messages[i].CreatedAt.Before(messages[j].CreatedAt)
	})
	return messages, nil
}

func validateMessage(message Message) error {
	if err := validateMessageID(message.ID); err != nil {
		return err
	}
	if err := validateMessageAddress("recipient", message.To); err != nil {
		return err
	}
	if err := validateMessageAddress("sender", message.From); err != nil {
		return err
	}
	if strings.TrimSpace(message.Text) == "" || len(message.Text) > maxMessageTextBytes {
		return newError(KindUsage, "Message text must contain 1 through 16384 bytes", "Provide bounded non-empty text with --text")
	}
	if len(message.Artifact) > maxMessageArtifact {
		return newError(KindUsage, "Artifact reference exceeds 2048 bytes", "Use a shorter artifact reference")
	}
	if message.ReplyTo != "" {
		if err := validateMessageID(message.ReplyTo); err != nil {
			return err
		}
	}
	if message.AckedAt != nil || message.AckedBy != "" {
		return newError(KindUsage, "New messages cannot include acknowledgement state", "Send an unacknowledged message")
	}
	return nil
}

func validateMessageID(id string) error {
	if id == "" || len(id) > maxMessageIDBytes || !messageAddressPattern.MatchString(id) {
		return newError(KindUsage, "Invalid message ID", "Use a stable ID containing letters, digits, dots, underscores, colons, at signs, plus signs, or hyphens")
	}
	return nil
}

func validateMessageAddress(label, value string) error {
	if len(value) > maxMessageAddress || !messageAddressPattern.MatchString(value) {
		return newError(KindUsage, fmt.Sprintf("Invalid %s address", label), "Use a stable address containing letters, digits, dots, underscores, colons, at signs, plus signs, or hyphens")
	}
	return nil
}

func sameMessageInput(existing, requested Message) bool {
	return existing.ID == requested.ID && existing.To == requested.To && existing.From == requested.From && existing.Text == requested.Text && existing.ReplyTo == requested.ReplyTo && existing.Artifact == requested.Artifact
}

func decodeMessage(path string, data []byte) (Message, error) {
	var envelope messageEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil || envelope.SchemaVersion != messageSchemaVersion || envelope.Kind != "message" || envelope.ObservedAt.IsZero() {
		return Message{}, malformedError("Malformed message record", "Inspect and repair the message store")
	}
	message := envelope.Message
	if err := validateMessage(Message{ID: message.ID, To: message.To, From: message.From, CreatedAt: message.CreatedAt, Text: message.Text, ReplyTo: message.ReplyTo, Artifact: message.Artifact}); err != nil || message.CreatedAt.IsZero() {
		return Message{}, malformedError("Message record has invalid fields", "Inspect and repair the message store")
	}
	if message.AckedAt != nil {
		if message.AckedAt.IsZero() || validateMessageAddress("acknowledger", message.AckedBy) != nil {
			return Message{}, malformedError("Message acknowledgement is malformed", "Inspect and repair the message store")
		}
	} else if message.AckedBy != "" {
		return Message{}, malformedError("Message has an acknowledger without an acknowledgement time", "Inspect and repair the message store")
	}
	return message, nil
}

func (s *Store) messagesDir() string { return filepath.Join(s.root, "messages") }

func (s *Store) messagePath(id string) string {
	digest := sha256.Sum256([]byte(id))
	return filepath.Join(s.messagesDir(), hex.EncodeToString(digest[:])+".json")
}
