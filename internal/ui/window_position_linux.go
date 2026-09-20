package ui

import (
	"strings"
	"sync"

	"codeberg.org/puregotk/purego"
	"codeberg.org/puregotk/puregotk/v4/gdk"
)

var (
	x11Once                 sync.Once
	x11Available            bool
	gdkX11DisplayGetDisplay func(uintptr) uintptr
	gdkX11SurfaceGetXID     func(uintptr) uintptr
	xDefaultRootWindow      func(uintptr) uintptr
	xTranslateCoordinates   func(uintptr, uintptr, uintptr, int32, int32, *int32, *int32, *uintptr) int32
	xMoveWindow             func(uintptr, uintptr, int32, int32) int32
	xFlush                  func(uintptr) int32
)

func loadX11Positioning() {
	x11Once.Do(func() {
		x11, err := purego.Dlopen("libX11.so.6", purego.RTLD_NOW|purego.RTLD_LOCAL)
		if err != nil {
			return
		}
		for _, symbol := range []string{
			"gdk_x11_display_get_xdisplay", "gdk_x11_surface_get_xid",
			"XDefaultRootWindow", "XTranslateCoordinates", "XMoveWindow", "XFlush",
		} {
			handle := uintptr(purego.RTLD_DEFAULT)
			if strings.HasPrefix(symbol, "X") {
				handle = x11
			}
			if _, err := purego.Dlsym(handle, symbol); err != nil {
				return
			}
		}
		purego.RegisterLibFunc(&gdkX11DisplayGetDisplay, purego.RTLD_DEFAULT, "gdk_x11_display_get_xdisplay")
		purego.RegisterLibFunc(&gdkX11SurfaceGetXID, purego.RTLD_DEFAULT, "gdk_x11_surface_get_xid")
		purego.RegisterLibFunc(&xDefaultRootWindow, x11, "XDefaultRootWindow")
		purego.RegisterLibFunc(&xTranslateCoordinates, x11, "XTranslateCoordinates")
		purego.RegisterLibFunc(&xMoveWindow, x11, "XMoveWindow")
		purego.RegisterLibFunc(&xFlush, x11, "XFlush")
		x11Available = true
	})
}

func x11Window(surface *gdk.Surface) (display uintptr, xid uintptr, ok bool) {
	if surface == nil {
		return 0, 0, false
	}
	gdkDisplay := surface.GetDisplay()
	if gdkDisplay == nil || !strings.Contains(gdkDisplay.GetName(), ":") {
		return 0, 0, false
	}
	loadX11Positioning()
	if !x11Available {
		return 0, 0, false
	}
	display = gdkX11DisplayGetDisplay(gdkDisplay.GoPointer())
	xid = gdkX11SurfaceGetXID(surface.GoPointer())
	return display, xid, display != 0 && xid != 0
}

func windowPosition(surface *gdk.Surface) (int, int, bool) {
	display, xid, ok := x11Window(surface)
	if !ok {
		return 0, 0, false
	}
	root := xDefaultRootWindow(display)
	var x, y int32
	var child uintptr
	if xTranslateCoordinates(display, xid, root, 0, 0, &x, &y, &child) == 0 {
		return 0, 0, false
	}
	return int(x), int(y), true
}

func moveWindowTo(surface *gdk.Surface, x, y int) bool {
	display, xid, ok := x11Window(surface)
	if !ok {
		return false
	}
	xMoveWindow(display, xid, int32(x), int32(y))
	xFlush(display)
	return true
}

func windowPositionVisible(surface *gdk.Surface, x, y, width, height int) bool {
	if surface == nil || width <= 0 || height <= 0 {
		return false
	}
	display := surface.GetDisplay()
	if display == nil {
		return false
	}
	monitors := display.GetMonitors()
	if monitors == nil {
		return false
	}
	windowLeft, windowTop := int64(x), int64(y)
	windowRight := windowLeft + int64(width)
	windowBottom := windowTop + int64(height)
	for i := uint32(0); i < monitors.GetNItems(); i++ {
		ptr := monitors.GetItem(i)
		if ptr == 0 {
			continue
		}
		monitor := gdk.MonitorNewFromInternalPtr(ptr)
		var geometry gdk.Rectangle
		monitor.GetGeometry(&geometry)
		monitor.Unref()
		monitorLeft, monitorTop := int64(geometry.X), int64(geometry.Y)
		monitorRight := monitorLeft + int64(geometry.Width)
		monitorBottom := monitorTop + int64(geometry.Height)
		if windowLeft < monitorRight && windowRight > monitorLeft && windowTop < monitorBottom && windowBottom > monitorTop {
			return true
		}
	}
	return false
}
