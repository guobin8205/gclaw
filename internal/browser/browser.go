package browser

import (
	"context"
)

// Browser defines the interface for browser automation backends.
type Browser interface {
	// Navigate opens the given URL in the browser.
	Navigate(ctx context.Context, url string) error

	// Snapshot extracts the accessibility tree text representation of the
	// current page. Interactive elements are annotated with @ref identifiers
	// (e.g. @e1, @e2) so that subsequent Click/Type calls can target them.
	Snapshot(ctx context.Context) (string, error)

	// Click clicks the element identified by ref (e.g. "@e3").
	Click(ctx context.Context, ref string) error

	// Type clears the element identified by ref and types text into it.
	Type(ctx context.Context, ref string, text string) error

	// Scroll scrolls the page in the given direction ("up" or "down") by
	// the specified number of viewport-height units.
	Scroll(ctx context.Context, direction string, amount int) error

	// Press dispatches a key press event (e.g. "Enter", "Tab", "Escape").
	Press(ctx context.Context, key string) error

	// Screenshot captures a full-page screenshot as PNG bytes.
	Screenshot(ctx context.Context) ([]byte, error)

	// Close releases all browser resources.
	Close() error
}
