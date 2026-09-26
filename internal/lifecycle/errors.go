package lifecycle

import "github.com/akofink/akagent-cli/internal/store"

// validationError preserves the existing internal category and recovery text for
// lifecycle validation failures until the CLI contract explicitly changes.
func validationError(message string) error {
	return &store.Error{Kind: store.KindInternal, Message: message}
}
