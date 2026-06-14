package cli

import (
	"errors"
	"fmt"
	"io"

	"github.com/tpisel/memento/internal/ignore"
	"github.com/tpisel/memento/internal/manifest"
	"github.com/tpisel/memento/internal/markdown"
	"github.com/tpisel/memento/internal/note"
	"github.com/tpisel/memento/internal/vault"
)

var (
	ErrInvalidArguments      = errors.New("invalid arguments")
	ErrUnknownCommand        = errors.New("unknown command")
	ErrInvalidEntryReference = errors.New("invalid entry reference")
	ErrNumericOutOfRange     = errors.New("numeric reference out of range")
	ErrIO                    = errors.New("I/O error")
)

func printCLIError(stderr io.Writer, verb string, err error) {
	token, hint := errorToken(err), errorHint(err)
	fmt.Fprintf(stderr, "memento %s: %s: %v\n", verb, token, err)
	if hint != "" {
		fmt.Fprintln(stderr, hint)
	}
}

func printRootError(stderr io.Writer, err error) {
	token, hint := errorToken(err), errorHint(err)
	fmt.Fprintf(stderr, "memento: %s: %v\n", token, err)
	if hint != "" {
		fmt.Fprintln(stderr, hint)
	}
}

func errorToken(err error) string {
	switch {
	case errors.Is(err, ErrUnknownCommand):
		return "unknown-command"
	case errors.Is(err, ErrInvalidArguments):
		return "invalid-arguments"
	case errors.Is(err, ErrInvalidEntryReference):
		return "invalid-entry-reference"
	case errors.Is(err, ErrNumericOutOfRange):
		return "numeric-out-of-range"
	case errors.Is(err, vault.ErrVaultNotFound):
		return "vault-not-found"
	case errors.Is(err, vault.ErrMultipleVaults):
		return "multiple-vaults"
	case errors.Is(err, manifest.ErrNotFound):
		return "manifest-not-found"
	case errors.Is(err, manifest.ErrInvalid):
		return "manifest-invalid"
	case errors.Is(err, manifest.ErrSchemaUnsupported):
		return "manifest-schema-unsupported"
	case errors.Is(err, manifest.ErrStale):
		return "manifest-stale"
	case errors.Is(err, note.ErrInvalidKey):
		return "invalid-key"
	case errors.Is(err, note.ErrSectionNotFound):
		return "section-not-found"
	case errors.Is(err, note.ErrNotFound):
		return "key-not-found"
	case errors.Is(err, note.ErrUnsupportedWriteOperation):
		return "unsupported-write-operation"
	case errors.Is(err, note.ErrReadOnly):
		return "mode-rejects-write"
	case errors.Is(err, ignore.ErrUnsupportedNegation),
		errors.Is(err, ignore.ErrEmptyPattern),
		errors.Is(err, ignore.ErrEmptySegment),
		errors.Is(err, ignore.ErrInvalidRecursiveWildcard):
		return "ignore-file-invalid"
	case errors.Is(err, markdown.ErrMalformedFrontmatter),
		errors.Is(err, markdown.ErrUnterminatedFrontmatter),
		errors.Is(err, markdown.ErrInvalidMode),
		errors.Is(err, markdown.ErrInvalidUpdated):
		return "frontmatter-invalid"
	case errors.Is(err, ErrIO):
		return "io-error"
	default:
		return "io-error"
	}
}

func errorHint(err error) string {
	switch {
	case errors.Is(err, manifest.ErrNotFound):
		return "run: memento compile"
	case errors.Is(err, manifest.ErrStale):
		return "run: memento compile && memento brief\nnote: entry numbers will likely shift after compile."
	case errors.Is(err, ErrUnknownCommand), errors.Is(err, ErrInvalidArguments):
		return "Run 'memento help' for usage."
	default:
		return ""
	}
}
