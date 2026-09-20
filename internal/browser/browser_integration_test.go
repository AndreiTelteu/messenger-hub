//go:build integration

package browser

import (
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"

	"codeberg.org/puregotk/puregotk/v4/gio"
	"codeberg.org/puregotk/puregotk/v4/glib"
	"codeberg.org/puregotk/puregotk/v4/gtk"
	"codeberg.org/puregotk/puregotk/v4/webkit"
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
		view            *View
		dataDir         string
		cacheDir        string
		loaded          chan struct{}
		permissionState chan string
		notified        chan string
		propagated      chan string
		onPropagated    func(webkit.WebView, uintptr) bool
	}
	newFixture := func() fixture {
		dataDir, cacheDir := t.TempDir(), t.TempDir()
		loaded := make(chan struct{}, 1)
		permissionState := make(chan string, 1)
		notified := make(chan string, 1)
		propagated := make(chan string, 1)
		view, err := New(dataDir, cacheDir, Callbacks{Changed: func(title, uri string, loading bool) {
			if !loading && title == "browser integration" && uri == server.URL+"/" {
				select {
				case loaded <- struct{}{}:
				default:
				}
			}
			if !loading && strings.HasPrefix(title, "permission-") {
				select {
				case permissionState <- strings.TrimPrefix(title, "permission-"):
				default:
				}
			}
		}, Notification: func(_ uint64, title, body string) bool {
			notified <- title + "|" + body
			return false
		}})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		view.AllowNotificationOrigin(server.URL + "/")
		onPropagated := func(_ webkit.WebView, ptr uintptr) bool {
			notification := webkit.NotificationNewFromInternalPtr(ptr)
			propagated <- notification.GetTitle() + "|" + notification.GetBody()
			return true
		}
		view.view.ConnectShowNotification(&onPropagated)
		return fixture{view: view, dataDir: dataDir, cacheDir: cacheDir, loaded: loaded, permissionState: permissionState, notified: notified, propagated: propagated, onPropagated: onPropagated}
	}

	a, b := newFixture(), newFixture()
	settings := a.view.view.GetSettings()
	if !settings.GetEnableMediaStream() || !settings.GetEnableWebrtc() {
		settings.Unref()
		t.Fatal("media stream or WebRTC support is disabled")
	}
	settings.Unref()
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
	jsFinished := make(chan error, 1)
	jsCallback := gio.AsyncReadyCallback(func(_ uintptr, result uintptr, _ uintptr) {
		value, err := a.view.view.EvaluateJavascriptFinish(&gio.AsyncResultBase{Ptr: result})
		if value != nil {
			value.Unref()
		}
		jsFinished <- err
	})
	script := `void navigator.permissions.query({name: "notifications"}).then(status => { document.title = "permission-" + status.state + "-" + Notification.permission; }); "started";`
	a.view.view.EvaluateJavascript(script, -1, "", server.URL+"/", nil, &jsCallback, 0)
	waitFor(t, 15*time.Second, func() bool {
		select {
		case got := <-a.permissionState:
			if got != "granted-granted" {
				t.Fatalf("notification permission state = %q", got)
			}
			return true
		default:
			return false
		}
	})
	notificationFinished := make(chan error, 1)
	notificationCallback := gio.AsyncReadyCallback(func(_ uintptr, result uintptr, _ uintptr) {
		value, err := a.view.view.EvaluateJavascriptFinish(&gio.AsyncResultBase{Ptr: result})
		if value != nil {
			value.Unref()
		}
		notificationFinished <- err
	})
	a.view.view.EvaluateJavascript(`new Notification("Integration notification", {body: "ready"}); "shown";`, -1, "", server.URL+"/", nil, &notificationCallback, 0)
	waitFor(t, 15*time.Second, func() bool {
		select {
		case err := <-notificationFinished:
			if err != nil {
				t.Fatalf("show notification JavaScript: %v", err)
			}
			return true
		default:
			return false
		}
	})
	waitFor(t, 15*time.Second, func() bool {
		select {
		case err := <-jsFinished:
			if err != nil {
				t.Fatalf("evaluate notification JavaScript: %v", err)
			}
			return true
		default:
			return false
		}
	})
	waitFor(t, 15*time.Second, func() bool {
		select {
		case got := <-a.notified:
			if got != "Integration notification|ready" {
				t.Fatalf("notification = %q", got)
			}
			return true
		default:
			return false
		}
	})
	waitFor(t, 15*time.Second, func() bool {
		select {
		case got := <-a.propagated:
			if got != "Integration notification|ready" {
				t.Fatalf("propagated notification = %q", got)
			}
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
