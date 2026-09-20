//go:build integration

package browser

import (
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
	"time"

	"codeberg.org/puregotk/puregotk/v4/glib"
	"codeberg.org/puregotk/puregotk/v4/gtk"
)

// TestViewRuntime intentionally never creates or presents a GtkWindow. It may
// use the caller's DISPLAY, but all widgets remain children of an unpresented
// GtkStack. Run explicitly with: go test -tags integration ./internal/browser
func TestViewRuntime(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	gtk.Init()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!doctype html><title>browser integration</title><p>ready</p>`))
	}))
	defer server.Close()

	type fixture struct {
		view     *View
		dataDir  string
		cacheDir string
		loaded   chan struct{}
	}
	newFixture := func() fixture {
		dataDir, cacheDir := t.TempDir(), t.TempDir()
		loaded := make(chan struct{}, 1)
		view, err := New(dataDir, cacheDir, Callbacks{Changed: func(title, uri string, loading bool) {
			if !loading && title == "browser integration" && uri == server.URL+"/" {
				select {
				case loaded <- struct{}{}:
				default:
				}
			}
		}})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		return fixture{view: view, dataDir: dataDir, cacheDir: cacheDir, loaded: loaded}
	}

	a, b := newFixture(), newFixture()
	if a.view.session.GoPointer() == b.view.session.GoPointer() {
		t.Fatal("views share a network session")
	}
	if got := a.view.manager.GetBaseDataDirectory(); got != a.dataDir {
		t.Fatalf("data directory = %q, want %q", got, a.dataDir)
	}
	if got := a.view.manager.GetBaseCacheDirectory(); got != a.cacheDir {
		t.Fatalf("cache directory = %q, want %q", got, a.cacheDir)
	}
	if got := b.view.manager.GetBaseDataDirectory(); got != b.dataDir {
		t.Fatalf("second data directory = %q, want %q", got, b.dataDir)
	}
	if got := b.view.manager.GetBaseCacheDirectory(); got != b.cacheDir {
		t.Fatalf("second cache directory = %q, want %q", got, b.cacheDir)
	}
	stack := gtk.NewStack()
	aw, bw := a.view.Widget(), b.view.Widget()
	if aw == nil || bw == nil {
		t.Fatal("New returned a nil widget")
	}
	stack.AddNamed(aw, "a")
	stack.AddNamed(bw, "b")
	a.view.LoadURL(server.URL + "/")
	b.view.LoadURL(server.URL + "/")
	waitFor(t, 15*time.Second, func() bool {
		select {
		case <-a.loaded:
			return true
		default:
			return false
		}
	})
	waitFor(t, 15*time.Second, func() bool {
		select {
		case <-b.loaded:
			return true
		default:
			return false
		}
	})

	stack.Remove(bw)
	b.view.Close()
	stack.Remove(aw)
	cleared := make(chan error, 1)
	a.view.Clear(func(err error) { cleared <- err })
	waitFor(t, 15*time.Second, func() bool {
		select {
		case err := <-cleared:
			if err != nil {
				t.Fatalf("Clear: %v", err)
			}
			return true
		default:
			return false
		}
	})
	a.view.Close()

	// Reopening the same service directories after a terminal clear must create
	// a fresh session and a usable view.
	freshLoaded := make(chan struct{}, 1)
	fresh, err := New(a.dataDir, a.cacheDir, Callbacks{Changed: func(title, _ string, loading bool) {
		if !loading && title == "browser integration" {
			select {
			case freshLoaded <- struct{}{}:
			default:
			}
		}
	}})
	if err != nil {
		t.Fatalf("New after Clear: %v", err)
	}
	fw := fresh.Widget()
	stack.AddNamed(fw, "fresh")
	fresh.LoadURL(server.URL + "/")
	waitFor(t, 15*time.Second, func() bool {
		select {
		case <-freshLoaded:
			return true
		default:
			return false
		}
	})
	stack.Remove(fw)
	fresh.Close()
	stack.Unref()
}

func waitFor(t *testing.T, timeout time.Duration, ready func() bool) {
	t.Helper()
	context := glib.MainContextDefault()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for context.Pending() {
			context.Iteration(false)
		}
		if ready() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("timed out while pumping the GLib main context")
}
