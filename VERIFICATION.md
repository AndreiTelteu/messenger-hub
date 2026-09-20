# Linux verification

Verified on CachyOS with GNOME Wayland, GTK 4.22.5, WebKitGTK 2.52.6, Go 1.27.1, and Linux amd64. All interactive checks used isolated temporary profiles and local fake data.

## Automated checks

- `make build` and `make check`
- `go test -race ./internal/model`
- `go test -tags integration ./internal/browser -run TestViewRuntime -v`
- `desktop-file-validate packaging/io.github.messengerhub.MessengerHub.desktop`
- Complete installer run against an isolated temporary prefix

## Observed native UI scenarios

- Added multiple services and verified independent cookies and local storage.
- Closed and reopened the app; local sessions and service order persisted.
- Disabled and re-enabled a service; its WebView closed and reopened with its profile preserved.
- Cleared one service profile without affecting another instance.
- Opened an authentication popup using the profile of its originating service.
- Reordered services and verified that `Ctrl+1` through `Ctrl+9` followed the saved order.
- Verified empty-name validation without corrupting the saved service.
- Loaded the official WhatsApp and Telegram web clients to their QR screens.
- Verified the compact shell at the 800×600 minimum and the 1100×720 default size.
- Verified right-click, Menu, and Shift+F10 service actions.
- Verified accessible service names and disabled-state labels.
- Verified readable disabled and empty states on the dark surface.
- Verified Web Notification permission and delivery through the WebKit integration callback.
- Verified that MediaStream and WebRTC are enabled on each WebView.
- Verified that the per-service notification preference defaults on and persists when disabled.
- Verified safe window-size restoration and X11 fallback when saved coordinates are offscreen.

No messages were sent and no calls were started. Microsoft Teams was verified with an existing signed-in Chrome profile: app mode loaded without the WebKit call error or notification banner, and Chrome delivered the Teams permission confirmation through GNOME.
