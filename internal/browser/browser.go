// Package browser provides the small WebKitGTK surface used by the application.
// All methods and callbacks must be used from the GTK main thread.
package browser

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"codeberg.org/puregotk/puregotk/v4/gdk"
	"codeberg.org/puregotk/puregotk/v4/gio"
	"codeberg.org/puregotk/puregotk/v4/glib"
	"codeberg.org/puregotk/puregotk/v4/gobject"
	"codeberg.org/puregotk/puregotk/v4/gtk"
	"codeberg.org/puregotk/puregotk/v4/webkit"
)

type Callbacks struct {
	Changed func(title, uri string, loading bool)
	Favicon func(*gdk.Texture)
	Error   func(string)

	// Permission asks the application to present a user decision. respond must
	// be called on the GTK thread. If Permission is nil, requests are denied.
	Permission func(kind, origin string, respond func(allow bool))
}

type popup struct {
	view   *webkit.WebView
	window *gtk.Window
}

type View struct {
	mu       sync.Mutex
	view     *webkit.WebView
	session  *webkit.NetworkSession
	manager  *webkit.WebsiteDataManager
	favicons *webkit.FaviconDatabase
	popups   map[uintptr]*popup
	closed   bool
	clearing bool
	cb       Callbacks

	// Signal callbacks are retained because the generated bindings identify
	// callbacks using the address of the Go function value.
	onLoad       func(webkit.WebView, webkit.LoadEvent)
	onLoadFailed func(webkit.WebView, webkit.LoadEvent, string, uintptr) bool
	onTerminated func(webkit.WebView, webkit.WebProcessTerminationReason)
	onNotify     func(gobject.Object, uintptr)
	onFavicon    func(webkit.FaviconDatabase, string, string)
	onIconReady  gio.AsyncReadyCallback
	onCreate     func(webkit.WebView, uintptr) gtk.Widget
	onPermission func(webkit.WebView, uintptr) bool
}

func New(dataDir, cacheDir string, callbacks Callbacks) (*View, error) {
	if dataDir == "" || cacheDir == "" {
		return nil, errors.New("browser: data and cache directories are required")
	}
	for _, dir := range []string{dataDir, cacheDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("browser: create %q: %w", dir, err)
		}
	}

	session := webkit.NewNetworkSession(dataDir, cacheDir)
	if session == nil {
		return nil, errors.New("browser: could not create WebKit network session")
	}
	cookies := session.GetCookieManager()
	if cookies == nil {
		session.Unref()
		return nil, errors.New("browser: could not get WebKit cookie manager")
	}
	// This must happen before constructing a view or starting any navigation.
	cookies.SetPersistentStorage(filepath.Join(dataDir, "cookies.sqlite"), webkit.CookiePersistentStorageSqliteValue)
	cookies.SetAcceptPolicy(webkit.CookiePolicyAcceptAlwaysValue)
	cookies.Unref()

	obj := gobject.NewObject(webkit.WebViewGLibType(), "network-session", session.GoPointer(), uintptr(0))
	if obj == nil {
		session.Unref()
		return nil, errors.New("browser: could not construct WebKit view")
	}
	// Sink the widget's floating reference so View owns it independently of
	// whichever GTK container embeds it.
	gobject.IncreaseRef(obj.GoPointer())
	wv := webkit.WebViewNewFromInternalPtr(obj.GoPointer())
	manager := session.GetWebsiteDataManager()
	if manager == nil {
		wv.Unref()
		session.Unref()
		return nil, errors.New("browser: could not get website data manager")
	}
	manager.SetFaviconsEnabled(true)
	favicons := manager.GetFaviconDatabase()
	v := &View{
		view: wv, session: session, manager: manager, favicons: favicons,
		popups: make(map[uintptr]*popup), cb: callbacks,
	}
	if favicons != nil {
		v.onIconReady = func(source, result, _ uintptr) {
			database := webkit.FaviconDatabaseNewFromInternalPtr(source)
			texture, err := database.GetFaviconFinish(&gio.AsyncResultBase{Ptr: result})
			if err != nil || texture == nil {
				return
			}
			v.emitFavicon(texture)
			texture.Unref()
		}
		v.onFavicon = func(database webkit.FaviconDatabase, pageURI, _ string) {
			database.GetFavicon(pageURI, nil, &v.onIconReady, 0)
		}
		favicons.ConnectFaviconChanged(&v.onFavicon)
	}
	v.connect(wv, true)
	return v, nil
}

func (v *View) Widget() *gtk.Widget {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.closed || v.view == nil {
		return nil
	}
	return gtk.WidgetNewFromInternalPtr(v.view.GoPointer())
}

func (v *View) LoadURL(uri string) {
	if w := v.webView(); w != nil {
		w.LoadUri(uri)
	}
}
func (v *View) Reload() {
	if w := v.webView(); w != nil {
		w.Reload()
	}
}
func (v *View) GoBack() {
	if w := v.webView(); w != nil && w.CanGoBack() {
		w.GoBack()
	}
}
func (v *View) GoForward() {
	if w := v.webView(); w != nil && w.CanGoForward() {
		w.GoForward()
	}
}

func (v *View) webView() *webkit.WebView {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.closed {
		return nil
	}
	return v.view
}

func (v *View) connect(w *webkit.WebView, primary bool) {
	if primary {
		v.onLoad = func(w webkit.WebView, _ webkit.LoadEvent) { v.changed(&w) }
		v.onLoadFailed = func(_ webkit.WebView, _ webkit.LoadEvent, uri string, _ uintptr) bool {
			v.report("failed to load " + uri)
			return false // retain WebKit's useful error page
		}
		v.onTerminated = func(_ webkit.WebView, reason webkit.WebProcessTerminationReason) {
			v.report(fmt.Sprintf("web process terminated (reason %d)", reason))
		}
		w.ConnectLoadChanged(&v.onLoad)
		v.onNotify = func(_ gobject.Object, _ uintptr) { v.changed(w) }
		w.ConnectNotify(&v.onNotify)
		w.ConnectLoadFailed(&v.onLoadFailed)
		w.ConnectWebProcessTerminated(&v.onTerminated)
	}

	v.onCreate = func(parent webkit.WebView, _ uintptr) gtk.Widget {
		obj := gobject.NewObject(webkit.WebViewGLibType(), "related-view", parent.GoPointer(), uintptr(0))
		if obj == nil {
			v.report("could not create authentication window")
			return gtk.Widget{}
		}
		gobject.IncreaseRef(obj.GoPointer())
		child := webkit.WebViewNewFromInternalPtr(obj.GoPointer())
		win := gtk.NewWindow()
		win.SetDefaultSize(900, 700)
		win.SetTitle("Sign in")
		win.SetChild(gtk.WidgetNewFromInternalPtr(child.GoPointer()))
		p := &popup{view: child, window: win}
		v.mu.Lock()
		v.popups[child.GoPointer()] = p
		v.mu.Unlock()
		var ready func(webkit.WebView)
		ready = func(_ webkit.WebView) { win.Present() }
		var closeView func(webkit.WebView)
		closeView = func(_ webkit.WebView) { v.closePopup(child.GoPointer()) }
		var closeWin func(gtk.Window) bool
		closeWin = func(_ gtk.Window) bool { v.closePopup(child.GoPointer()); return false }
		child.ConnectReadyToShow(&ready)
		child.ConnectClose(&closeView)
		child.ConnectCreate(&v.onCreate)
		child.ConnectPermissionRequest(&v.onPermission)
		win.ConnectCloseRequest(&closeWin)
		return *gtk.WidgetNewFromInternalPtr(child.GoPointer())
	}
	w.ConnectCreate(&v.onCreate)

	v.onPermission = func(w webkit.WebView, request uintptr) bool {
		return v.requestPermission(&w, request)
	}
	w.ConnectPermissionRequest(&v.onPermission)
	// Returning false from run-file-chooser leaves WebKitGTK's native chooser
	// active. Downloads likewise use WebKitGTK's standard download handling.
}

func (v *View) changed(w *webkit.WebView) {
	v.mu.Lock()
	closed, changed, favicon := v.closed, v.cb.Changed, v.cb.Favicon
	v.mu.Unlock()
	if closed {
		return
	}
	if changed != nil {
		changed(w.GetTitle(), w.GetUri(), w.GetPropertyIsLoading())
	}
	if favicon != nil {
		if texture := w.GetFavicon(); texture != nil {
			favicon(texture)
		}
	}
}

func (v *View) emitFavicon(texture *gdk.Texture) {
	v.mu.Lock()
	closed, clearing, callback := v.closed, v.clearing, v.cb.Favicon
	v.mu.Unlock()
	if !closed && !clearing && callback != nil {
		callback(texture)
	}
}

func (v *View) report(message string) {
	v.mu.Lock()
	closed, cb := v.closed, v.cb.Error
	v.mu.Unlock()
	if !closed && cb != nil {
		cb(message)
	}
}

func (v *View) requestPermission(w *webkit.WebView, ptr uintptr) bool {
	// The generated signal exposes the interface as an opaque pointer. Keep the
	// prompt conservative instead of guessing its concrete GObject type.
	kind := "camera, microphone, screen sharing, or notifications"

	v.mu.Lock()
	handler, closed := v.cb.Permission, v.closed
	v.mu.Unlock()
	if closed || handler == nil {
		denyPermission(ptr)
		return true
	}
	gobject.IncreaseRef(ptr)
	var once sync.Once
	handler(kind, w.GetUri(), func(allow bool) {
		once.Do(func() {
			v.mu.Lock()
			dead := v.closed
			v.mu.Unlock()
			if allow && !dead {
				allowPermission(ptr)
			} else {
				denyPermission(ptr)
			}
			(&gobject.Object{Ptr: ptr}).Unref()
		})
	})
	return true
}

func allowPermission(ptr uintptr) { webkit.XWebkitPermissionRequestAllow(ptr) }
func denyPermission(ptr uintptr)  { webkit.XWebkitPermissionRequestDeny(ptr) }

func (v *View) closePopup(ptr uintptr) {
	v.mu.Lock()
	p := v.popups[ptr]
	delete(v.popups, ptr)
	v.mu.Unlock()
	if p == nil {
		return
	}
	p.view.StopLoading()
	p.window.Destroy()
	p.view.Unref()
}

func (v *View) Close() {
	v.mu.Lock()
	if v.closed {
		v.mu.Unlock()
		return
	}
	v.closed = true
	w, session, manager, favicons := v.view, v.session, v.manager, v.favicons
	v.view, v.session, v.manager, v.favicons = nil, nil, nil, nil
	popups := v.popups
	v.popups = make(map[uintptr]*popup)
	v.mu.Unlock()
	if w != nil {
		w.StopLoading()
		w.Unref()
	}
	for _, p := range popups {
		p.view.StopLoading()
		p.window.Destroy()
		p.view.Unref()
	}
	if manager != nil {
		manager.Unref()
	}
	if favicons != nil {
		favicons.Unref()
	}
	if session != nil {
		session.Unref()
	}
}

// Clear permanently stops and releases all pages, then asynchronously removes
// every website-data type. The caller must first remove Widget from its GTK
// container and create a new View after completion. done runs on the GTK thread.
func (v *View) Clear(done func(error)) {
	v.mu.Lock()
	if v.closed {
		v.mu.Unlock()
		if done != nil {
			done(errors.New("browser: view is closed"))
		}
		return
	}
	if v.clearing {
		v.mu.Unlock()
		if done != nil {
			done(errors.New("browser: clear already in progress"))
		}
		return
	}
	v.clearing = true
	w, manager := v.view, v.manager
	v.view = nil
	popups := v.popups
	v.popups = make(map[uintptr]*popup)
	v.mu.Unlock()
	for _, p := range popups {
		p.view.StopLoading()
		p.window.Destroy()
		p.view.Unref()
	}
	if w != nil {
		w.StopLoading()
		w.Unref()
	}
	if manager == nil {
		v.finishClear(done, errors.New("browser: website data manager unavailable"))
		return
	}
	// Close may run before WebKit invokes the completion callback.
	gobject.IncreaseRef(manager.GoPointer())
	callback := gio.AsyncReadyCallback(func(_ uintptr, result uintptr, _ uintptr) {
		_, err := manager.ClearFinish(&gio.AsyncResultBase{Ptr: result})
		manager.Unref()
		v.finishClear(done, err)
	})
	manager.Clear(webkit.WebsiteDataAllValue, glib.TimeSpan(0), nil, &callback, 0)
}

func (v *View) finishClear(done func(error), err error) {
	v.mu.Lock()
	v.clearing = false
	v.mu.Unlock()
	if done != nil {
		done(err)
	}
}
