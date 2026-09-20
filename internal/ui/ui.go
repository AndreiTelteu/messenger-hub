package ui

import (
	"fmt"
	"log"
	"math"
	"net/url"
	"os"
	"strings"

	"codeberg.org/puregotk/puregotk/v4/gdk"
	"codeberg.org/puregotk/puregotk/v4/gio"
	"codeberg.org/puregotk/puregotk/v4/glib"
	"codeberg.org/puregotk/puregotk/v4/gobject"
	"codeberg.org/puregotk/puregotk/v4/gtk"
	"messenger-hub/internal/browser"
	"messenger-hub/internal/model"
)

const appID = "io.github.messengerhub.MessengerHub"

type controller struct {
	app              *gtk.Application
	window           *gtk.ApplicationWindow
	store            *model.Store
	state            model.State
	list             *gtk.ListBox
	stack            *gtk.Stack
	status           *gtk.Label
	views            map[string]*browser.View
	pages            map[string]*gtk.Widget
	rows             map[uintptr]string
	favicons         map[string]*gdk.Texture
	notifications    map[string]map[string]struct{}
	selectedID       string
	clearing         map[string]bool
	clearingPages    map[string]*gtk.Widget
	disabledPages    map[string]*gtk.Widget
	emptyPage        *gtk.Widget
	syncingSelection bool
	closing          bool
}

// Run starts the native GTK application. It must be called from the process's
// initial, OS-thread-locked goroutine.
func Run(store *model.Store, state model.State) int {
	app := gtk.NewApplication(appID, gio.GApplicationNonUniqueValue)
	c := &controller{app: app, store: store, state: state, views: map[string]*browser.View{}, pages: map[string]*gtk.Widget{}, rows: map[uintptr]string{}, favicons: map[string]*gdk.Texture{}, notifications: map[string]map[string]struct{}{}, clearing: map[string]bool{}, clearingPages: map[string]*gtk.Widget{}, disabledPages: map[string]*gtk.Widget{}}
	activate := func(_ gio.Application) { c.activate() }
	app.ConnectActivate(&activate)
	return int(app.Run(1, []string{"messenger-hub"}))
}

func (c *controller) activate() {
	if c.window != nil {
		c.window.Present()
		return
	}
	if settings := gtk.SettingsGetDefault(); settings != nil {
		settings.SetPropertyGtkApplicationPreferDarkTheme(true)
	}
	c.window = gtk.NewApplicationWindow(c.app)
	c.window.SetTitle("Messenger Hub")
	width, height := 1100, 720
	if c.state.Window.Width >= 800 && c.state.Window.Height >= 600 {
		width, height = c.state.Window.Width, c.state.Window.Height
	}
	c.window.SetDefaultSize(int32(width), int32(height))
	c.window.SetSizeRequest(800, 600)
	c.installStyles()

	root := gtk.NewBox(gtk.OrientationHorizontalValue, 0)
	root.AddCssClass("app-shell")
	side := gtk.NewBox(gtk.OrientationVerticalValue, 0)
	side.AddCssClass("service-rail")
	side.SetSizeRequest(68, -1)
	side.SetHexpand(false)
	c.list = gtk.NewListBox()
	c.list.SetSelectionMode(gtk.SelectionSingleValue)
	c.list.AddCssClass("service-list")
	c.installSidebarDnD()
	scroll := gtk.NewScrolledWindow()
	scroll.SetChild(&c.list.Widget)
	scroll.SetPolicy(gtk.PolicyNeverValue, gtk.PolicyAutomaticValue)
	scroll.SetVexpand(true)
	scroll.SetHexpand(true)
	side.Append(&scroll.Widget)
	add := gtk.NewButtonFromIconName("list-add-symbolic")
	add.AddCssClass("rail-action")
	add.SetTooltipText("Add service (Ctrl+N)")
	clickedAdd := func(gtk.Button) { c.showAddDialog() }
	add.ConnectClicked(&clickedAdd)
	side.Append(&add.Widget)

	main := gtk.NewOverlay()
	main.SetHexpand(true)
	main.SetVexpand(true)
	c.stack = gtk.NewStack()
	c.stack.SetHexpand(true)
	c.stack.SetVexpand(true)
	main.SetChild(&c.stack.Widget)
	c.status = gtk.NewLabel("")
	c.status.AddCssClass("status-toast")
	c.status.SetHalign(gtk.AlignCenterValue)
	c.status.SetValign(gtk.AlignEndValue)
	c.status.SetMarginBottom(18)
	c.status.SetVisible(false)
	main.AddOverlay(&c.status.Widget)
	root.Append(&side.Widget)
	root.Append(&main.Widget)
	c.window.SetChild(&root.Widget)
	selected := func(_ gtk.ListBox, ptr uintptr) {
		if c.syncingSelection {
			return
		}
		if id := c.rows[ptr]; id != "" {
			c.selectID(id)
		}
	}
	c.list.ConnectRowSelected(&selected)
	c.installActions()
	c.rebuildSidebar()
	if c.state.ActiveID != "" {
		c.selectID(c.state.ActiveID)
	} else {
		c.showEmpty()
	}
	closeRequest := func(gtk.Window) bool { c.close(); return false }
	c.window.ConnectCloseRequest(&closeRequest)
	c.window.Present()
	if c.state.Window.Maximized {
		c.window.Maximize()
	} else if c.state.Window.Positioned {
		x, y := c.state.Window.X, c.state.Window.Y
		idle := glib.SourceOnceFunc(func(uintptr) {
			if !c.closing && windowPositionVisible(c.window.GetSurface(), x, y, width, height) {
				moveWindowTo(c.window.GetSurface(), x, y)
			}
		})
		glib.IdleAddOnce(&idle, 0)
	}
}

func (c *controller) installStyles() {
	provider := gtk.NewCssProvider()
	provider.LoadFromString(`
.app-shell { background: #1f2126; }
.service-rail {
  background: #24262b;
  border-right: 1px solid #111216;
  padding: 8px 0;
}
.service-list { background: transparent; color: #f5f6f8; }
.service-list row {
  min-height: 54px;
  padding: 5px 8px 5px 7px;
  border-left: 1px solid transparent;
}
.service-list row:hover { background: #2d3037; }
.service-list row:selected {
  background: #343740;
  border-left-color: #8b6cff;
}
.service-list row.disabled-service image { opacity: 0.38; }
.service-icon { -gtk-icon-size: 32px; }
.rail-action {
  min-width: 42px;
  min-height: 42px;
  margin: 8px 10px 2px;
  color: #d7d9df;
  background: transparent;
  border: 0;
  border-radius: 10px;
  box-shadow: none;
}
.rail-action:hover { color: #ffffff; background: #343740; }
.service-menu { padding: 6px; }
.service-menu button {
  min-width: 150px;
  padding: 7px 12px;
  border-radius: 7px;
}
.status-toast {
  padding: 7px 12px;
  color: #ffffff;
  background: rgba(20, 21, 25, 0.92);
  border-radius: 8px;
}
.empty-state {
  padding: 36px;
  color: #f5f6f8;
  background: #1f2126;
}
.empty-state .dim-label { color: #b7bac3; }
`)
	display := gdk.DisplayGetDefault()
	if display != nil {
		gtk.StyleContextAddProviderForDisplay(display, provider, 600)
	}
}

func (c *controller) installActions() {
	addAction := func(name string, fn func(), accelerators ...string) {
		action := gio.NewSimpleAction(name, nil)
		activate := func(gio.SimpleAction, uintptr) { fn() }
		action.ConnectActivate(&activate)
		c.app.AddAction(action)
		if len(accelerators) > 0 {
			c.app.SetAccelsForAction("app."+name, accelerators)
		}
	}
	addAction("add-service", c.showAddDialog, "<Control>n")
	addAction("reload", c.reloadActive, "<Control>r")
	for i := 1; i <= 9; i++ {
		position := i
		addAction(fmt.Sprintf("select-%d", i), func() { c.selectPosition(position) }, fmt.Sprintf("<Control>%d", i))
	}

	// The controller is kept as a direct fallback for compositors which do not
	// dispatch application accelerators while focus is inside a WebKit widget.
	keys := gtk.NewEventControllerKey()
	keys.SetPropagationPhase(gtk.PhaseCaptureValue)
	pressed := func(_ gtk.EventControllerKey, key uint32, _ uint32, state gdk.ModifierType) bool {
		if state&gdk.ControlMaskValue == 0 {
			return false
		}
		switch key {
		case uint32(gdk.KEY_n):
			c.showAddDialog()
			return true
		case uint32(gdk.KEY_r):
			c.reloadActive()
			return true
		case uint32(gdk.KEY_1), uint32(gdk.KEY_2), uint32(gdk.KEY_3), uint32(gdk.KEY_4), uint32(gdk.KEY_5), uint32(gdk.KEY_6), uint32(gdk.KEY_7), uint32(gdk.KEY_8), uint32(gdk.KEY_9):
			c.selectPosition(int(key-uint32(gdk.KEY_1)) + 1)
			return true
		}
		return false
	}
	keys.ConnectKeyPressed(&pressed)
	c.window.AddController(&keys.EventController)

	menuKeys := gtk.NewEventControllerKey()
	menuPressed := func(_ gtk.EventControllerKey, key uint32, _ uint32, state gdk.ModifierType) bool {
		if key != uint32(gdk.KEY_Menu) && (key != uint32(gdk.KEY_F10) || state&gdk.ShiftMaskValue == 0) {
			return false
		}
		c.openSelectedServiceMenu()
		return true
	}
	menuKeys.ConnectKeyPressed(&menuPressed)
	c.list.AddController(&menuKeys.EventController)
}

func (c *controller) rebuildSidebar() {
	c.syncingSelection = true
	defer func() { c.syncingSelection = false }()
	c.list.RemoveAll()
	c.rows = map[uintptr]string{}
	highlightID := c.selectedID
	if _, ok := c.service(highlightID); !ok {
		highlightID = c.state.ActiveID
	}
	for _, s := range c.state.Services {
		row := gtk.NewListBoxRow()
		row.SetName("service-" + s.ID)
		accessibleName := s.Name
		if !s.Enabled {
			accessibleName += ", disabled"
		}
		row.SetTooltipText(accessibleName)
		row.UpdateProperty(gtk.AccessiblePropertyLabelValue, accessibleName, -1)
		box := gtk.NewBox(gtk.OrientationHorizontalValue, 0)
		box.SetHalign(gtk.AlignCenterValue)
		box.SetValign(gtk.AlignCenterValue)
		icon := c.serviceIcon(s)
		box.Append(&icon.Widget)
		if !s.Enabled {
			row.AddCssClass("disabled-service")
		}
		row.SetChild(&box.Widget)
		c.installServiceMenu(row, s)
		box.Unref()
		c.list.Append(&row.Widget)
		c.rows[row.GoPointer()] = s.ID
		if s.ID == highlightID {
			c.list.SelectRow(row)
		}
		row.Unref()
	}
}

func (c *controller) serviceIcon(s model.Service) *gtk.Image {
	texture := c.favicons[s.ID]
	if texture == nil {
		if cacheDir, err := c.store.CacheProfileDir(s.ID); err == nil {
			if loaded, loadErr := gdk.NewTextureFromFilename(cacheDir + "/favicon.png"); loadErr == nil {
				texture = loaded
				c.favicons[s.ID] = loaded
			}
		}
	}
	var icon *gtk.Image
	if texture != nil {
		icon = gtk.NewImageFromPaintable(texture)
	} else {
		icon = gtk.NewImageFromIconName("web-browser-symbolic")
	}
	icon.SetPixelSize(32)
	icon.AddCssClass("service-icon")
	return icon
}

func (c *controller) updateFavicon(id string, texture *gdk.Texture) {
	if c.closing || texture == nil {
		return
	}
	if current := c.favicons[id]; current != nil && current.GoPointer() == texture.GoPointer() {
		return
	}
	gobject.IncreaseRef(texture.GoPointer())
	if old := c.favicons[id]; old != nil {
		old.Unref()
	}
	c.favicons[id] = texture
	if cacheDir, err := c.store.CacheProfileDir(id); err == nil {
		_ = os.MkdirAll(cacheDir, 0o700)
		texture.SaveToPng(cacheDir + "/favicon.png")
	}
	c.rebuildSidebar()
}

func (c *controller) installServiceMenu(row *gtk.ListBoxRow, service model.Service) {
	gesture := gtk.NewGestureClick()
	gesture.SetButton(3)
	gesture.SetPropagationPhase(gtk.PhaseCaptureValue)
	pressed := func(_ gtk.GestureClick, _ int32, x, y float64) {
		c.showServiceMenu(row, service, x, y)
	}
	gesture.ConnectPressed(&pressed)
	row.AddController(&gesture.EventController)
}

func (c *controller) showServiceMenu(row *gtk.ListBoxRow, service model.Service, x, y float64) {
	popover := gtk.NewPopover()
	popover.SetAutohide(true)
	popover.SetHasArrow(false)
	popover.SetPosition(gtk.PosRightValue)
	popover.SetParent(&row.Widget)
	closed := func(gtk.Popover) { popover.Unparent() }
	popover.ConnectClosed(&closed)
	menu := gtk.NewBox(gtk.OrientationVerticalValue, 2)
	menu.AddCssClass("service-menu")
	popover.SetChild(&menu.Widget)

	var firstButton *gtk.Button
	addItem := func(label string, sensitive bool, action func()) {
		button := gtk.NewButtonWithLabel(label)
		if firstButton == nil {
			firstButton = button
		}
		button.SetHalign(gtk.AlignFillValue)
		button.SetSensitive(sensitive)
		clicked := func(gtk.Button) {
			popover.Popdown()
			action()
		}
		button.ConnectClicked(&clicked)
		menu.Append(&button.Widget)
	}
	busy := c.clearing[service.ID]
	if service.Enabled {
		addItem("Disable", !busy, func() { c.setEnabled(service.ID, false) })
	} else {
		addItem("Enable", !busy, func() { c.setEnabled(service.ID, true) })
	}
	addItem("Refresh", service.Enabled && !busy, func() { c.reloadService(service.ID) })
	addItem("Home", service.Enabled && !busy, func() { c.goHome(service.ID) })
	addItem("Manage", !busy, func() { c.showManageDialogFor(service.ID) })
	popover.SetPointingTo(&gdk.Rectangle{X: int32(x), Y: int32(y), Width: 1, Height: 1})
	popover.Popup()
	if firstButton != nil {
		firstButton.GrabFocus()
	}
}

func (c *controller) openSelectedServiceMenu() {
	row := c.list.GetSelectedRow()
	if row == nil {
		return
	}
	defer row.Unref()
	id := c.rows[row.GoPointer()]
	service, ok := c.service(id)
	if !ok {
		return
	}
	c.showServiceMenu(row, service, 34, 27)
}

func (c *controller) reloadService(id string) {
	c.selectID(id)
	if view := c.views[id]; view != nil {
		view.Reload()
	}
}

func (c *controller) goHome(id string) {
	service, ok := c.service(id)
	if !ok || !service.Enabled {
		return
	}
	c.selectID(id)
	if view := c.views[id]; view != nil {
		view.LoadURL(service.URL)
	}
}

func (c *controller) installSidebarDnD() {
	gesture := gtk.NewGestureDrag()
	gesture.SetButton(1)
	gesture.SetPropagationPhase(gtk.PhaseCaptureValue)
	debugDnD("install list-gesture button=%d", gesture.GetButton())
	var sourceID string
	var startY float64
	dragBegin := func(_ gtk.GestureDrag, _ float64, y float64) {
		sourceID = ""
		startY = y
		row := c.list.GetRowAtY(int32(startY))
		if row == nil {
			debugDnD("gesture-begin y=%.1f row=<nil>", y)
			return
		}
		sourceID = c.rows[row.GoPointer()]
		if sourceID != "" {
			row.AddCssClass("dragging")
		}
		row.Unref()
		debugDnD("gesture-begin y=%.1f source=%s", y, sourceID)
	}
	dragUpdate := func(_ gtk.GestureDrag, _ float64, dy float64) {
		debugDnD("gesture-update source=%s dy=%.1f", sourceID, dy)
	}
	dragEnd := func(_ gtk.GestureDrag, _ float64, dy float64) {
		if sourceID == "" {
			return
		}
		for i, service := range c.state.Services {
			if service.ID == sourceID {
				if row := c.list.GetRowAtIndex(int32(i)); row != nil {
					row.RemoveCssClass("dragging")
					row.Unref()
				}
				break
			}
		}
		if math.Abs(dy) < 8 {
			debugDnD("gesture-end source=%s dy=%.1f click", sourceID, dy)
			c.selectID(sourceID)
			return
		}
		targetY := startY + dy
		row := c.list.GetRowAtY(int32(targetY))
		if row == nil {
			debugDnD("gesture-end source=%s target-y=%.1f row=<nil>", sourceID, targetY)
			return
		}
		id := c.rows[row.GoPointer()]
		row.Unref()
		if id == "" {
			debugDnD("gesture-end source=%s target-y=%.1f target=<unknown>", sourceID, targetY)
			return
		}
		debugDnD("gesture-end source=%s target=%s dy=%.1f", sourceID, id, dy)
		accepted := c.moveOnto(sourceID, id)
		debugDnD("gesture-result target=%s source=%s accepted=%t", id, sourceID, accepted)
	}
	gesture.ConnectDragBegin(&dragBegin)
	gesture.ConnectDragUpdate(&dragUpdate)
	gesture.ConnectDragEnd(&dragEnd)
	c.list.AddController(&gesture.EventController)
}

func debugDnD(format string, args ...any) {
	if os.Getenv("MESSENGER_HUB_DEBUG_DND") == "1" {
		log.Printf("messenger-hub dnd: "+format, args...)
	}
}

// moveOnto gives the dragged service the target row's one-based position.
func (c *controller) moveOnto(source, target string) bool {
	if source == target {
		return false
	}
	pos := 0
	for i, s := range c.state.Services {
		if s.ID == target {
			pos = i + 1
			break
		}
	}
	if pos == 0 {
		return false
	}
	if err := c.state.Move(source, pos); err != nil {
		c.error(err)
		return false
	}
	c.persist()
	// Waiting for idle lets GTK finish dispatching the gesture before rows are
	// destroyed and rebuilt.
	idle := glib.SourceOnceFunc(func(uintptr) {
		if !c.closing {
			c.rebuildSidebar()
		}
	})
	glib.IdleAddOnce(&idle, 0)
	return true
}

func (c *controller) selectPosition(pos int) {
	if pos < 1 || pos > len(c.state.Services) {
		return
	}
	c.selectID(c.state.Services[pos-1].ID)
	if view := c.activeView(); view != nil && view.Widget() != nil {
		view.Widget().GrabFocus()
	} else {
		c.stack.GrabFocus()
	}
}

func (c *controller) selectID(id string) {
	s, ok := c.service(id)
	if !ok {
		return
	}
	c.selectedID = id
	c.syncSidebarSelection(id)
	if c.clearing[id] {
		c.showClearing(s)
		return
	}
	if !s.Enabled {
		c.showDisabled(s)
		return
	}
	if c.state.ActiveID != id {
		c.state.ActiveID = id
		c.persist()
	}
	if _, ok := c.views[id]; !ok {
		if !c.createView(s) {
			return
		}
	}
	if page := c.pages[id]; page != nil {
		c.stack.SetVisibleChild(page)
	}
	c.setStatus("")
}

func (c *controller) syncSidebarSelection(id string) {
	if c.syncingSelection || c.list == nil {
		return
	}
	var wanted *gtk.ListBoxRow
	for i, service := range c.state.Services {
		if service.ID == id {
			wanted = c.list.GetRowAtIndex(int32(i))
			break
		}
	}
	if wanted == nil {
		return
	}
	current := c.list.GetSelectedRow()
	if current != nil && current.GoPointer() == wanted.GoPointer() {
		current.Unref()
		wanted.Unref()
		return
	}
	c.syncingSelection = true
	c.list.SelectRow(wanted)
	c.syncingSelection = false
	if current != nil {
		current.Unref()
	}
	wanted.Unref()
}

func (c *controller) createView(s model.Service) bool {
	if c.closing || c.clearing[s.ID] || c.views[s.ID] != nil {
		return c.views[s.ID] != nil
	}
	data, derr := c.store.DataProfileDir(s.ID)
	cache, cerr := c.store.CacheProfileDir(s.ID)
	if derr != nil || cerr != nil {
		c.error(fmt.Errorf("invalid profile"))
		return false
	}
	callbacks := browser.Callbacks{}
	callbacks.Changed = func(_ string, _ string, loading bool) {
		if c.closing || c.selectedID != s.ID {
			return
		}
		if loading {
			c.setStatus("Loading…")
		} else {
			c.setStatus("")
		}
	}
	callbacks.Favicon = func(texture *gdk.Texture) { c.updateFavicon(s.ID, texture) }
	callbacks.Notification = func(id uint64, title, body string) bool {
		return c.showDesktopNotification(s.ID, id, title, body)
	}
	callbacks.NotificationClosed = func(id uint64) { c.withdrawDesktopNotification(s.ID, id) }
	callbacks.Error = func(message string) {
		if c.closing {
			return
		}
		c.setStatus("Error: " + message + " — right-click the service and choose Refresh.")
	}
	callbacks.Permission = func(kind, origin string, respond func(bool)) { c.askPermission(kind, origin, respond) }
	v, err := browser.New(data, cache, callbacks)
	if err != nil {
		c.error(err)
		return false
	}
	if teamsWebURL(s.URL) {
		// Teams rejects calls when WebKitGTK identifies itself as Safari on
		// Linux, even though the required WebRTC and H.264 support is present.
		// Match the locally supported Chromium generation before first load.
		v.SetUserAgent("Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/153.0.0.0 Safari/537.36")
	}
	c.views[s.ID] = v
	c.pages[s.ID] = v.Widget()
	c.stack.AddNamed(v.Widget(), s.ID)
	v.LoadURL(s.URL)
	return true
}

func teamsWebURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "teams.microsoft.com" || strings.HasSuffix(host, ".teams.microsoft.com") ||
		host == "teams.cloud.microsoft" || strings.HasSuffix(host, ".teams.cloud.microsoft")
}

func (c *controller) showEmpty() {
	if c.emptyPage == nil {
		page := emptyPage("Your services, one quiet workspace", "Add a service to begin.")
		add := gtk.NewButtonWithLabel("Add a service")
		cb := func(gtk.Button) { c.showAddDialog() }
		add.ConnectClicked(&cb)
		page.Append(&add.Widget)
		c.emptyPage = &page.Widget
		c.stack.AddNamed(c.emptyPage, "empty")
	}
	c.stack.SetVisibleChild(c.emptyPage)
	c.setStatus("")
}
func (c *controller) showDisabled(s model.Service) {
	page := c.disabledPages[s.ID]
	if page == nil {
		box := emptyPage(s.Name+" is disabled", "Its profile is preserved while the browser stays closed.")
		enable := gtk.NewButtonWithLabel("Enable")
		cb := func(gtk.Button) { c.setEnabled(s.ID, true) }
		enable.ConnectClicked(&cb)
		box.Append(&enable.Widget)
		page = &box.Widget
		c.disabledPages[s.ID] = page
		c.stack.AddNamed(page, "disabled-"+s.ID)
	}
	c.stack.SetVisibleChild(page)
	c.setStatus("")
}

func (c *controller) showClearing(s model.Service) {
	page := c.clearingPages[s.ID]
	if page == nil {
		box := emptyPage("Clearing data for "+s.Name, "The service will reopen when its local profile has been cleared.")
		page = &box.Widget
		c.clearingPages[s.ID] = page
		c.stack.AddNamed(page, "clearing-"+s.ID)
	}
	c.stack.SetVisibleChild(page)
	c.setStatus("Clearing local data…")
}
func emptyPage(title, body string) *gtk.Box {
	b := gtk.NewBox(gtk.OrientationVerticalValue, 12)
	b.AddCssClass("empty-state")
	b.SetHalign(gtk.AlignCenterValue)
	b.SetValign(gtk.AlignCenterValue)
	h := gtk.NewLabel(title)
	h.AddCssClass("title-1")
	h.SetMaxWidthChars(52)
	h.SetWrap(true)
	p := gtk.NewLabel(body)
	p.AddCssClass("dim-label")
	p.SetMaxWidthChars(52)
	p.SetWrap(true)
	b.Append(&h.Widget)
	b.Append(&p.Widget)
	return b
}

func (c *controller) showAddDialog() {
	d := c.dialog("Add service", "Cancel", "Add")
	area := d.GetContentArea()
	search := gtk.NewSearchEntry()
	search.SetPlaceholderText("Search the catalog")
	searchLabel := gtk.NewLabelWithMnemonic("_Search the catalog")
	searchLabel.SetXalign(0)
	searchLabel.SetMnemonicWidget(&search.Widget)
	area.Append(&searchLabel.Widget)
	area.Append(&search.Widget)
	combo := gtk.NewComboBoxText()
	fillCatalog := func(query string) {
		combo.RemoveAll()
		query = strings.ToLower(strings.TrimSpace(query))
		for _, p := range model.Catalog {
			if query == "" || strings.Contains(strings.ToLower(p.Name+" "+p.Description), query) {
				combo.Append(p.ID, p.Name+" — "+p.Description)
			}
		}
		combo.SetActive(0)
	}
	fillCatalog("")
	changed := func(gtk.SearchEntry) { fillCatalog(search.GetText()) }
	search.ConnectSearchChanged(&changed)
	catalogLabel := gtk.NewLabelWithMnemonic("_Service")
	catalogLabel.SetXalign(0)
	catalogLabel.SetMnemonicWidget(&combo.Widget)
	area.Append(&catalogLabel.Widget)
	area.Append(&combo.Widget)
	name := gtk.NewEntry()
	name.SetPlaceholderText("Name (for a custom service)")
	nameLabel := gtk.NewLabelWithMnemonic("Display _name")
	nameLabel.SetXalign(0)
	nameLabel.SetMnemonicWidget(&name.Widget)
	area.Append(&nameLabel.Widget)
	area.Append(&name.Widget)
	url := gtk.NewEntry()
	url.SetPlaceholderText("https://… (optional for catalog services)")
	urlLabel := gtk.NewLabelWithMnemonic("_Web address")
	urlLabel.SetXalign(0)
	urlLabel.SetMnemonicWidget(&url.Widget)
	area.Append(&urlLabel.Widget)
	area.Append(&url.Widget)
	inlineError := gtk.NewLabel("")
	inlineError.SetXalign(0)
	inlineError.SetWrap(true)
	inlineError.AddCssClass("error")
	area.Append(&inlineError.Widget)
	resp := func(_ gtk.Dialog, id int32) {
		if id == int32(gtk.ResponseOkValue) {
			preset, ok := model.PresetByID(combo.GetActiveId())
			if !ok {
				if strings.TrimSpace(url.GetText()) == "" {
					inlineError.SetLabel("Choose a catalog service or enter a custom web address.")
					return
				}
				preset = model.Preset{ID: "custom", Name: "Custom service", URL: url.GetText()}
			}
			n, u, k := preset.Name, preset.URL, preset.ID
			if preset.ID == "custom" && strings.TrimSpace(url.GetText()) == "" {
				inlineError.SetLabel("Enter the web address for the custom service.")
				return
			}
			if strings.TrimSpace(name.GetText()) != "" {
				n = name.GetText()
			}
			if strings.TrimSpace(url.GetText()) != "" {
				u = url.GetText()
			}
			svc, err := model.NewService(n, k, u)
			if err != nil {
				inlineError.SetLabel(err.Error())
				return
			}
			if err = c.state.Add(svc); err != nil {
				inlineError.SetLabel(err.Error())
				return
			}
			c.persist()
			c.rebuildSidebar()
			c.selectID(svc.ID)
		}
		d.Destroy()
	}
	d.ConnectResponse(&resp)
	d.Present()
}

func (c *controller) showManageDialog() {
	c.showManageDialogFor(c.selectedID)
}

func (c *controller) showManageDialogFor(serviceID string) {
	if c.clearing[serviceID] {
		c.setStatus("Wait for data clearing to finish before managing this service.")
		return
	}
	s, ok := c.service(serviceID)
	if !ok {
		s, ok = c.state.Active()
	}
	if !ok && len(c.state.Services) > 0 {
		s = c.state.Services[0]
		ok = true
	}
	if !ok {
		return
	}
	d := c.dialog("Manage "+s.Name, "Close", "Save")
	area := d.GetContentArea()
	name := gtk.NewEntry()
	name.SetText(s.Name)
	name.SetPlaceholderText("Name")
	nameLabel := gtk.NewLabelWithMnemonic("Display _name")
	nameLabel.SetXalign(0)
	nameLabel.SetMnemonicWidget(&name.Widget)
	area.Append(&nameLabel.Widget)
	area.Append(&name.Widget)
	url := gtk.NewEntry()
	url.SetText(s.URL)
	url.SetPlaceholderText("URL")
	urlLabel := gtk.NewLabelWithMnemonic("_Web address")
	urlLabel.SetXalign(0)
	urlLabel.SetMnemonicWidget(&url.Widget)
	area.Append(&urlLabel.Widget)
	area.Append(&url.Widget)
	enabled := gtk.NewCheckButtonWithLabel("Service enabled")
	enabled.SetActive(s.Enabled)
	area.Append(&enabled.Widget)
	notifications := gtk.NewCheckButtonWithLabel("Desktop notifications")
	notifications.SetActive(s.NotificationsEnabled())
	notifications.SetTooltipText("Show this service's web notifications in GNOME")
	area.Append(&notifications.Widget)
	inlineError := gtk.NewLabel("")
	inlineError.SetXalign(0)
	inlineError.SetWrap(true)
	inlineError.AddCssClass("error")
	area.Append(&inlineError.Widget)
	buttons := gtk.NewBox(gtk.OrientationHorizontalValue, 6)
	up := gtk.NewButtonWithLabel("Move up")
	down := gtk.NewButtonWithLabel("Move down")
	clear := gtk.NewButtonWithLabel("Clear data")
	remove := gtk.NewButtonWithLabel("Remove")
	remove.AddCssClass("destructive-action")
	for _, b := range []*gtk.Button{up, down, clear, remove} {
		buttons.Append(&b.Widget)
	}
	area.Append(&buttons.Widget)
	upcb := func(gtk.Button) { c.moveRelative(s.ID, -1); d.Destroy() }
	downcb := func(gtk.Button) { c.moveRelative(s.ID, 1); d.Destroy() }
	up.ConnectClicked(&upcb)
	down.ConnectClicked(&downcb)
	clearcb := func(gtk.Button) {
		c.confirm("Clear local data?", "Sign-in details and website data for this service will be deleted.", func() {
			d.Destroy()
			c.clearData(s.ID)
		})
	}
	clear.ConnectClicked(&clearcb)
	removecb := func(gtk.Button) {
		c.confirm("Remove "+s.Name+"?", "The service will leave the rail. Its profile data will remain on disk.", func() { d.Destroy(); c.remove(s.ID) })
	}
	remove.ConnectClicked(&removecb)
	resp := func(_ gtk.Dialog, id int32) {
		if id == int32(gtk.ResponseOkValue) {
			trimmedName := strings.TrimSpace(name.GetText())
			if trimmedName == "" {
				inlineError.SetLabel("Enter a name for the service.")
				return
			}
			normalized, err := model.NormalizeURL(url.GetText())
			if err != nil {
				inlineError.SetLabel(err.Error())
				return
			}
			candidate := c.state
			candidate.Services = append([]model.Service(nil), c.state.Services...)
			for i := range candidate.Services {
				if candidate.Services[i].ID == s.ID {
					candidate.Services[i].Name = trimmedName
					candidate.Services[i].URL = normalized
					break
				}
			}
			if err := candidate.SetEnabled(s.ID, enabled.GetActive()); err != nil {
				inlineError.SetLabel(err.Error())
				return
			}
			if err := candidate.SetNotifications(s.ID, notifications.GetActive()); err != nil {
				inlineError.SetLabel(err.Error())
				return
			}
			if err := c.store.Save(candidate); err != nil {
				inlineError.SetLabel(err.Error())
				return
			}
			urlChanged := normalized != s.URL
			c.state = candidate
			if !notifications.GetActive() {
				c.withdrawServiceNotifications(s.ID)
			}
			// Recreate a disabled placeholder on its next presentation so a
			// renamed service cannot retain stale text.
			c.removeDisabledPage(s.ID)
			if !enabled.GetActive() {
				c.destroyView(s.ID)
			} else if urlChanged {
				if view := c.views[s.ID]; view != nil {
					view.LoadURL(normalized)
				}
			}
			c.rebuildSidebar()
			if enabled.GetActive() {
				c.selectID(s.ID)
			} else if c.state.ActiveID != "" {
				c.selectID(c.state.ActiveID)
			} else {
				c.showEmpty()
			}
		}
		d.Destroy()
	}
	d.ConnectResponse(&resp)
	d.Present()
}

func (c *controller) dialog(title, cancel, ok string) *gtk.Dialog {
	d := gtk.NewDialog()
	d.SetTitle(title)
	d.SetTransientFor(&c.window.Window)
	d.SetModal(true)
	d.SetDestroyWithParent(true)
	d.SetDefaultSize(480, -1)
	d.AddButton(cancel, int32(gtk.ResponseCancelValue))
	d.AddButton(ok, int32(gtk.ResponseOkValue))
	a := d.GetContentArea()
	a.SetSpacing(10)
	a.SetMarginTop(16)
	a.SetMarginBottom(16)
	a.SetMarginStart(16)
	a.SetMarginEnd(16)
	return d
}
func (c *controller) confirm(title, body string, yes func()) {
	d := c.dialog(title, "Cancel", "Confirm")
	l := gtk.NewLabel(body)
	l.SetWrap(true)
	d.GetContentArea().Append(&l.Widget)
	cb := func(_ gtk.Dialog, id int32) {
		if id == int32(gtk.ResponseOkValue) {
			yes()
		}
		d.Destroy()
	}
	d.ConnectResponse(&cb)
	d.Present()
}
func (c *controller) askPermission(kind, origin string, respond func(bool)) {
	d := c.dialog("Permission requested", "Block", "Allow")
	l := gtk.NewLabel(fmt.Sprintf("%s is requesting access to %s.", origin, kind))
	l.SetWrap(true)
	d.GetContentArea().Append(&l.Widget)
	cb := func(_ gtk.Dialog, id int32) { respond(id == int32(gtk.ResponseOkValue)); d.Destroy() }
	d.ConnectResponse(&cb)
	d.Present()
}

func (c *controller) showDesktopNotification(serviceID string, webID uint64, title, body string) bool {
	service, ok := c.service(serviceID)
	if !ok || !service.NotificationsEnabled() {
		return false
	}
	if strings.TrimSpace(title) == "" {
		title = service.Name
	}
	notification := gio.NewNotification(title)
	if notification == nil {
		return false
	}
	if body != "" {
		notification.SetBody(body)
	}
	notification.SetCategory("im.received")
	notification.SetPriority(gio.GNotificationPriorityNormalValue)
	key := desktopNotificationKey(serviceID, webID)
	c.app.SendNotification(key, notification)
	notification.Unref()
	if c.notifications[serviceID] == nil {
		c.notifications[serviceID] = map[string]struct{}{}
	}
	c.notifications[serviceID][key] = struct{}{}
	return true
}

func (c *controller) withdrawDesktopNotification(serviceID string, webID uint64) {
	key := desktopNotificationKey(serviceID, webID)
	c.app.WithdrawNotification(key)
	delete(c.notifications[serviceID], key)
	if len(c.notifications[serviceID]) == 0 {
		delete(c.notifications, serviceID)
	}
}

func (c *controller) withdrawServiceNotifications(serviceID string) {
	for key := range c.notifications[serviceID] {
		c.app.WithdrawNotification(key)
	}
	delete(c.notifications, serviceID)
}

func desktopNotificationKey(serviceID string, webID uint64) string {
	return fmt.Sprintf("service-%s-%d", serviceID, webID)
}

func (c *controller) setEnabled(id string, enabled bool) {
	if c.clearing[id] {
		c.setStatus("Wait for data clearing to finish before changing this service.")
		return
	}
	_ = c.state.SetEnabled(id, enabled)
	if enabled {
		c.removeDisabledPage(id)
	}
	if !enabled {
		c.withdrawServiceNotifications(id)
		c.destroyView(id)
	}
	c.persist()
	c.rebuildSidebar()
	if enabled {
		c.selectID(id)
	} else if c.state.ActiveID != "" {
		c.selectID(c.state.ActiveID)
	} else {
		c.showEmpty()
	}
}
func (c *controller) moveRelative(id string, delta int) {
	idx := -1
	for i, s := range c.state.Services {
		if s.ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return
	}
	pos := idx + 1 + delta
	if pos < 1 || pos > len(c.state.Services) {
		return
	}
	_ = c.state.Move(id, pos)
	c.persist()
	c.rebuildSidebar()
}
func (c *controller) remove(id string) {
	if c.clearing[id] {
		c.setStatus("Wait for data clearing to finish before removing this service.")
		return
	}
	c.destroyView(id)
	c.withdrawServiceNotifications(id)
	c.removeDisabledPage(id)
	if page := c.clearingPages[id]; page != nil {
		c.stack.Remove(page)
		delete(c.clearingPages, id)
	}
	if err := c.state.Remove(id); err != nil {
		c.error(err)
		return
	}
	c.persist()
	c.rebuildSidebar()
	if c.state.ActiveID != "" {
		c.selectID(c.state.ActiveID)
	} else {
		c.showEmpty()
	}
}

func (c *controller) removeDisabledPage(id string) {
	if page := c.disabledPages[id]; page != nil {
		c.stack.Remove(page)
		delete(c.disabledPages, id)
	}
}
func (c *controller) clearData(id string) {
	if c.clearing[id] {
		c.setStatus("This service's local data is already being cleared.")
		return
	}
	s, ok := c.service(id)
	if !ok {
		return
	}
	v := c.views[id]
	if v == nil {
		data, derr := c.store.DataProfileDir(id)
		cache, cerr := c.store.CacheProfileDir(id)
		if derr != nil || cerr != nil {
			c.error(fmt.Errorf("invalid profile"))
			return
		}
		var err error
		// A never-opened or disabled service still owns persistent WebKit data.
		// Build one detached view solely to obtain its profile data manager.
		v, err = browser.New(data, cache, browser.Callbacks{})
		if err != nil {
			c.error(err)
			return
		}
	}
	c.clearing[id] = true
	// WebKit's website-data manager can only clear after every page has been
	// stopped. Remove the widget first so GTK cannot dispatch into it while the
	// asynchronous clear is running.
	if page := c.pages[id]; page != nil {
		c.stack.Remove(page)
	}
	delete(c.views, id)
	delete(c.pages, id)
	c.showClearing(s)
	v.Clear(func(err error) {
		v.Close()
		delete(c.clearing, id)
		if page := c.clearingPages[id]; page != nil {
			c.stack.Remove(page)
			delete(c.clearingPages, id)
		}
		if c.closing {
			return
		}
		if err != nil {
			c.error(err)
		} else {
			c.setStatus("Local data cleared.")
		}
		if s, ok := c.service(id); ok {
			if s.Enabled {
				created := c.createView(s)
				if c.selectedID == id && created {
					if err == nil {
						c.selectID(id)
					} else if c.pages[id] != nil {
						c.stack.SetVisibleChild(c.pages[id])
					}
				}
			} else if c.selectedID == id {
				c.showDisabled(s)
			}
		}
	})
}
func (c *controller) destroyView(id string) {
	if v := c.views[id]; v != nil {
		if p := c.pages[id]; p != nil {
			c.stack.Remove(p)
		}
		v.Close()
		delete(c.views, id)
		delete(c.pages, id)
	}
}
func (c *controller) close() {
	if c.closing {
		return
	}
	c.rememberWindowGeometry()
	c.persist()
	c.closing = true
	for id := range c.views {
		c.destroyView(id)
	}
	for id, texture := range c.favicons {
		texture.Unref()
		delete(c.favicons, id)
	}
}
func (c *controller) rememberWindowGeometry() {
	if c.window == nil {
		return
	}
	c.state.Window.Maximized = c.window.IsMaximized()
	if c.state.Window.Maximized {
		return
	}
	width, height := int(c.window.GetWidth()), int(c.window.GetHeight())
	if width >= 800 && height >= 600 {
		c.state.Window.Width = width
		c.state.Window.Height = height
	}
	if x, y, ok := windowPosition(c.window.GetSurface()); ok {
		c.state.Window.X = x
		c.state.Window.Y = y
		c.state.Window.Positioned = true
	}
}
func (c *controller) reloadActive() {
	if v := c.activeView(); v != nil {
		v.Reload()
	}
}
func (c *controller) activeView() *browser.View {
	s, ok := c.service(c.selectedID)
	if !ok || !s.Enabled || c.clearing[c.selectedID] {
		return nil
	}
	return c.views[c.selectedID]
}
func (c *controller) service(id string) (model.Service, bool) {
	for _, s := range c.state.Services {
		if s.ID == id {
			return s, true
		}
	}
	return model.Service{}, false
}
func (c *controller) persist() {
	if err := c.store.Save(c.state); err != nil {
		c.error(err)
	}
}
func (c *controller) error(err error) { c.setStatus("Error: " + err.Error()) }
func (c *controller) setStatus(message string) {
	if c.status == nil {
		return
	}
	c.status.SetLabel(message)
	c.status.SetVisible(message != "")
}
