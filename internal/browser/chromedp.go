package browser

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"
)

// ChromedpBrowser implements the Browser interface using chromedp.
type ChromedpBrowser struct {
	allocCtx    context.Context
	allocCancel context.CancelFunc
	browserCtx  context.Context
	browserCancel context.CancelFunc
	mu          sync.Mutex
}

// NewChromedpBrowser creates a new headless Chromium instance.
func NewChromedpBrowser() (*ChromedpBrowser, error) {
	allocCtx, allocCancel := chromedp.NewContext(context.Background())

	// Start the browser.
	browserCtx, browserCancel := chromedp.NewContext(allocCtx)

	b := &ChromedpBrowser{
		allocCtx:    allocCtx,
		allocCancel: allocCancel,
		browserCtx:  browserCtx,
		browserCancel: browserCancel,
	}

	// Verify the browser started by navigating to about:blank.
	if err := b.Navigate(context.Background(), "about:blank"); err != nil {
		b.Close()
		return nil, fmt.Errorf("failed to start browser: %w", err)
	}

	return b, nil
}

// Navigate opens the given URL.
func (b *ChromedpBrowser) Navigate(ctx context.Context, url string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	return chromedp.Run(b.browserCtx,
		chromedp.Navigate(url),
		chromedp.WaitReady("body", chromedp.ByQuery),
	)
}

// snapshotJS is the JavaScript injected into the page to extract an
// accessibility-tree-like text representation. Every interactive element
// (links, buttons, inputs, selects, textareas, elements with role or
// tabindex) is annotated with a unique @ref identifier.
const snapshotJS = `
(() => {
  let counter = 0;
  const refMap = new WeakMap();

  function getRef(el) {
    if (refMap.has(el)) return refMap.get(el);
    const ref = '@e' + (++counter);
    el.setAttribute('data-browser-ref', ref);
    refMap.set(el, ref);
    return ref;
  }

  function isVisible(el) {
    if (!el) return false;
    const style = getComputedStyle(el);
    if (style.display === 'none' || style.visibility === 'hidden' || style.opacity === '0') return false;
    if (el.offsetWidth === 0 && el.offsetHeight === 0) return false;
    return true;
  }

  function isInteractive(el) {
    const tag = el.tagName.toLowerCase();
    const interactiveTags = new Set(['a','button','input','select','textarea','details','summary','option']);
    if (interactiveTags.has(tag)) return true;
    if (el.getAttribute('role')) return true;
    if (el.getAttribute('tabindex') !== null) return true;
    if (el.getAttribute('onclick')) return true;
    if (el.getAttribute('contenteditable') === 'true') return true;
    return false;
  }

  function walk(node, depth) {
    if (node.nodeType === Node.TEXT_NODE) {
      const text = node.textContent.trim();
      if (text) return '  '.repeat(depth) + text + '\n';
      return '';
    }
    if (node.nodeType !== Node.ELEMENT_NODE) return '';
    const el = node;
    if (!isVisible(el)) return '';

    let result = '';
    const tag = el.tagName.toLowerCase();

    if (isInteractive(el)) {
      const ref = getRef(el);
      const label = el.getAttribute('aria-label') || el.getAttribute('title') || el.getAttribute('placeholder') || '';
      const role = el.getAttribute('role') || tag;
      const type = el.getAttribute('type') || '';
      let desc = '';
      if (tag === 'input') {
        desc = type ? '[' + type + ' input]' : '[input]';
      } else if (tag === 'a') {
        desc = '[link]';
      } else if (tag === 'button') {
        desc = '[button]';
      } else if (tag === 'select') {
        desc = '[select]';
      } else if (tag === 'textarea') {
        desc = '[textarea]';
      } else {
        desc = '[' + role + ']';
      }
      const inlineText = (el.textContent || '').trim().substring(0, 80);
      let line = '  '.repeat(depth) + ref + ' ' + desc;
      if (label) line += ' label="' + label + '"';
      if (inlineText && tag !== 'input') line += ' "' + inlineText + '"';
      result += line + '\n';
    } else if (tag === 'img') {
      const ref = getRef(el);
      const alt = el.getAttribute('alt') || '';
      const src = (el.getAttribute('src') || '').substring(0, 60);
      result += '  '.repeat(depth) + ref + ' [img] alt="' + alt + '" src="' + src + '"\n';
    } else if (tag === 'h1' || tag === 'h2' || tag === 'h3' || tag === 'h4' || tag === 'h5' || tag === 'h6') {
      const text = (el.textContent || '').trim();
      if (text) result += '  '.repeat(depth) + '# ' + text + '\n';
    }

    for (const child of el.childNodes) {
      result += walk(child, depth + 1);
    }
    return result;
  }

  const body = document.body;
  if (!body) return '(empty page)';
  const output = walk(body, 0);
  return output || '(no visible content)';
})();
`

// Snapshot extracts the accessibility tree text from the current page.
func (b *ChromedpBrowser) Snapshot(ctx context.Context) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	var result string
	err := chromedp.Run(b.browserCtx,
		chromedp.Evaluate(snapshotJS, &result),
	)
	if err != nil {
		return "", fmt.Errorf("snapshot failed: %w", err)
	}
	return result, nil
}

// Click clicks the element identified by the given ref string (e.g. "@e3").
func (b *ChromedpBrowser) Click(ctx context.Context, ref string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	selector := fmt.Sprintf(`[data-browser-ref="%s"]`, ref)
	return chromedp.Run(b.browserCtx,
		chromedp.WaitVisible(selector, chromedp.ByQuery),
		chromedp.Click(selector, chromedp.ByQuery),
	)
}

// Type clears the element identified by ref and types text into it.
func (b *ChromedpBrowser) Type(ctx context.Context, ref string, text string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	selector := fmt.Sprintf(`[data-browser-ref="%s"]`, ref)
	return chromedp.Run(b.browserCtx,
		chromedp.WaitVisible(selector, chromedp.ByQuery),
		chromedp.Clear(selector, chromedp.ByQuery),
		chromedp.SendKeys(selector, text, chromedp.ByQuery),
	)
}

// Scroll scrolls the page in the given direction by the specified amount
// (in viewport-height units).
func (b *ChromedpBrowser) Scroll(ctx context.Context, direction string, amount int) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if amount <= 0 {
		amount = 1
	}

	var scrollY string
	dir := strings.ToLower(direction)
	switch dir {
	case "up":
		scrollY = fmt.Sprintf("window.scrollBy(0, -window.innerHeight * %d)", amount)
	case "down":
		scrollY = fmt.Sprintf("window.scrollBy(0, window.innerHeight * %d)", amount)
	default:
		return fmt.Errorf("unsupported scroll direction: %q (use 'up' or 'down')", direction)
	}

	return chromedp.Run(b.browserCtx,
		chromedp.Evaluate(scrollY, nil),
	)
}

// keyMapping translates common key names to chromedp key strings.
var keyMapping = map[string]string{
	"enter":  "\r",
	"return": "\r",
	"tab":    "\t",
	"escape": "\x1b",
	"esc":    "\x1b",
	"backspace": "\b",
	"delete":    "\x7f",
	"arrowup":    "",
	"arrowdown":  "",
	"arrowleft":  "",
	"arrowright": "",
	"up":    "",
	"down":  "",
	"left":  "",
	"right": "",
	"space": " ",
}

// Press dispatches a key press event.
func (b *ChromedpBrowser) Press(ctx context.Context, key string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	keyLower := strings.ToLower(key)
	if mapped, ok := keyMapping[keyLower]; ok {
		return chromedp.Run(b.browserCtx,
			chromedp.KeyEvent(mapped),
		)
	}

	// Dispatch via DOM for single printable characters or use input.dispatchKeyEvent.
	if len(key) == 1 {
		return chromedp.Run(b.browserCtx,
			chromedp.KeyEvent(key),
		)
	}

	// For multi-character key names (e.g. "Control+a"), use dispatchKeyEvent.
	return chromedp.Run(b.browserCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return input.DispatchKeyEvent(input.KeyDown).WithKey(key).Do(ctx)
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return input.DispatchKeyEvent(input.KeyUp).WithKey(key).Do(ctx)
		}),
	)
}

// Screenshot captures a full-page screenshot as PNG bytes.
func (b *ChromedpBrowser) Screenshot(ctx context.Context) ([]byte, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	var buf []byte
	err := chromedp.Run(b.browserCtx,
		chromedp.FullScreenshot(&buf, 100),
	)
	if err != nil {
		return nil, fmt.Errorf("screenshot failed: %w", err)
	}
	return buf, nil
}

// Close releases all browser resources.
func (b *ChromedpBrowser) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.browserCancel != nil {
		b.browserCancel()
	}
	if b.allocCancel != nil {
		b.allocCancel()
	}
	return nil
}
