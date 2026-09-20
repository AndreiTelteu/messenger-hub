// Package browser provides the small WebKitGTK surface used by the application.
// All methods and callbacks must be used from the GTK main thread.
package browser

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"codeberg.org/puregotk/purego"
	"codeberg.org/puregotk/puregotk/v4/gdk"
	"codeberg.org/puregotk/puregotk/v4/gio"
	"codeberg.org/puregotk/puregotk/v4/glib"
	"codeberg.org/puregotk/puregotk/v4/gobject"
	"codeberg.org/puregotk/puregotk/v4/gobject/types"
	"codeberg.org/puregotk/puregotk/v4/gtk"
	"codeberg.org/puregotk/puregotk/v4/webkit"
)

type Callbacks struct {
	Changed            func(title, uri string, loading bool)
	Favicon            func(*gdk.Texture)
	Error              func(string)
	Notification       func(id uint64, title, body string) bool
	NotificationClosed func(id uint64)

	// Permission asks the application to present a user decision. respond must
	// be called on the GTK thread. If Permission is nil, requests are denied.
	Permission func(kind, origin string, respond func(allow bool))
}

type popup struct {
	view   *webkit.WebView
	window *gtk.Window
}

type notificationContextRegistration struct {
	context      *webkit.WebContext
	onInitialize func(webkit.WebContext)
}

var notificationPermissions = struct {
	sync.Mutex
	origins  map[string]struct{}
	contexts map[uintptr]*notificationContextRegistration
}{origins: map[string]struct{}{}, contexts: map[uintptr]*notificationContextRegistration{}}

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
	onLoad         func(webkit.WebView, webkit.LoadEvent)
	onLoadFailed   func(webkit.WebView, webkit.LoadEvent, string, uintptr) bool
	onTerminated   func(webkit.WebView, webkit.WebProcessTerminationReason)
	onNotify       func(gobject.Object, uintptr)
	onFavicon      func(webkit.FaviconDatabase, string, string)
	onIconReady    gio.AsyncReadyCallback
	onCreate       func(webkit.WebView, uintptr) gtk.Widget
	onPermission   func(webkit.WebView, uintptr) bool
	onQueryState   func(webkit.WebView, uintptr) bool
	onNotification func(webkit.WebView, uintptr) bool
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
	settings := wv.GetSettings()
	settings.SetEnableMediaStream(true)
	settings.SetEnableWebrtc(true)
	settings.SetPropertyEnableMedia(true)
	settings.SetEnableWebaudio(true)
	settings.SetEnableEncryptedMedia(true)
	settings.Unref()
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

// SetUserAgent changes the identity sent by this isolated browser profile.
// Call it before the first navigation so feature detection is consistent for
// the lifetime of the page.
func (v *View) SetUserAgent(userAgent string) {
	if w := v.webView(); w != nil {
		settings := w.GetSettings()
		settings.SetUserAgent(userAgent)
		settings.Unref()
	}
}

// AllowNotificationOrigin makes Notification.permission immediately report
// "granted" for the origin. Some applications only inspect that property and
// never emit a permission request, so handling permission-request alone is not
// sufficient.
func (v *View) AllowNotificationOrigin(uri string) {
	origin := webkit.NewSecurityOriginForUri(uri)
	if origin == nil {
		return
	}
	key := origin.ToString()
	origin.Unref()
	if key == "" {
		return
	}
	w := v.webView()
	if w == nil {
		return
	}
	context := w.GetContext()
	if context == nil {
		return
	}

	notificationPermissions.Lock()
	notificationPermissions.origins[key] = struct{}{}
	registration := notificationPermissions.contexts[context.GoPointer()]
	if registration == nil {
		registration = &notificationContextRegistration{context: context}
		registration.onInitialize = func(current webkit.WebContext) {
			initializeNotificationPermissions(&current)
		}
		context.ConnectInitializeNotificationPermissions(&registration.onInitialize)
		notificationPermissions.contexts[context.GoPointer()] = registration
	} else {
		context.Unref()
	}
	notificationPermissions.Unlock()
	initializeNotificationPermissions(registration.context)
}

func initializeNotificationPermissions(context *webkit.WebContext) {
	notificationPermissions.Lock()
	keys := make([]string, 0, len(notificationPermissions.origins))
	for key := range notificationPermissions.origins {
		keys = append(keys, key)
	}
	notificationPermissions.Unlock()

	origins := make([]*webkit.SecurityOrigin, 0, len(keys))
	for _, key := range keys {
		if origin := webkit.NewSecurityOriginForUri(key); origin != nil {
			origins = append(origins, origin)
		}
	}
	if len(origins) == 0 {
		context.InitializeNotificationPermissions(nil, nil)
		return
	}
	nodes := make([]glib.List, len(origins))
	for i, origin := range origins {
		nodes[i].Data = origin.GoPointer()
		if i > 0 {
			nodes[i].Prev = &nodes[i-1]
		}
		if i+1 < len(nodes) {
			nodes[i].Next = &nodes[i+1]
		}
	}
	context.InitializeNotificationPermissions(&nodes[0], nil)
	for _, origin := range origins {
		origin.Unref()
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
	v.onQueryState = func(_ webkit.WebView, ptr uintptr) bool {
		if ptr == 0 || permissionStateQueryGetName(ptr) != "notifications" {
			return false
		}
		permissionStateQueryFinish(ptr, webkit.PermissionStateGrantedValue)
		return true
	}
	w.ConnectQueryPermissionState(&v.onQueryState)
	v.onNotification = func(_ webkit.WebView, ptr uintptr) bool {
		notification := webkit.NotificationNewFromInternalPtr(ptr)
		v.mu.Lock()
		handler, closedHandler, closed := v.cb.Notification, v.cb.NotificationClosed, v.closed
		v.mu.Unlock()
		if closed || handler == nil {
			return false
		}
		id := notification.GetId()
		handled := handler(id, notification.GetTitle(), notification.GetBody())
		if !handled {
			return false
		}
		if closedHandler != nil {
			onClosed := func(_ webkit.Notification) { closedHandler(id) }
			notification.ConnectClosed(&onClosed)
		}
		return true
	}
	w.ConnectShowNotification(&v.onNotification)
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
	if permissionIs(ptr, notificationPermissionRequestType()) {
		// The per-service preference controls delivery to the desktop. Keeping the
		// web permission granted lets the preference be toggled without forcing a
		// site-data reset or another website prompt.
		allowPermission(ptr)
		return true
	}

	kind := "the requested device"
	if permissionIs(ptr, webkit.UserMediaPermissionRequestGLibType()) {
		request := webkit.UserMediaPermissionRequestNewFromInternalPtr(ptr)
		var devices []string
		if webkit.UserMediaPermissionIsForAudioDevice(request) {
			devices = append(devices, "microphone")
		}
		if webkit.UserMediaPermissionIsForVideoDevice(request) {
			devices = append(devices, "camera")
		}
		if webkit.UserMediaPermissionIsForDisplayDevice(request) {
			devices = append(devices, "screen sharing")
		}
		if len(devices) > 0 {
			kind = strings.Join(devices, ", ")
		}
	} else if permissionIs(ptr, deviceInfoPermissionRequestType()) {
		kind = "camera and microphone device information"
	}

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

func permissionIs(ptr uintptr, target types.GType) bool {
	return ptr != 0 && typeCheckInstanceIsA(ptr, target)
}

var (
	typeCheckInstanceIsA              func(uintptr, types.GType) bool
	notificationPermissionRequestType func() types.GType
	deviceInfoPermissionRequestType   func() types.GType
	permissionStateQueryGetName       func(uintptr) string
	permissionStateQueryFinish        func(uintptr, webkit.PermissionState)
)

func init() {
	purego.RegisterLibFunc(&typeCheckInstanceIsA, purego.RTLD_DEFAULT, "g_type_check_instance_is_a")
	purego.RegisterLibFunc(&notificationPermissionRequestType, purego.RTLD_DEFAULT, "webkit_notification_permission_request_get_type")
	purego.RegisterLibFunc(&deviceInfoPermissionRequestType, purego.RTLD_DEFAULT, "webkit_device_info_permission_request_get_type")
	purego.RegisterLibFunc(&permissionStateQueryGetName, purego.RTLD_DEFAULT, "webkit_permission_state_query_get_name")
	purego.RegisterLibFunc(&permissionStateQueryFinish, purego.RTLD_DEFAULT, "webkit_permission_state_query_finish")
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
