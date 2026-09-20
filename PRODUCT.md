# Messenger Hub

<!-- impeccable:product-schema 1 -->

## Platform
Native Linux desktop (GTK4).

## Stack
Go, GTK4 and the system WebKitGTK 6. No Electron or Chromium.

## Users
The primary user is the owner of the project, who uses Messenger Hub daily to keep personal messaging and web communication services together in one native Linux desktop application.

## Product Purpose
Collect messaging web applications in one desktop window, with an independent persistent browser profile for each service instance.

## Capabilities and Constraints
Presets for WhatsApp, Microsoft Teams, Messenger, Instagram Direct, Telegram and other web applications, plus arbitrary HTTP(S) URLs. Multiple accounts of the same service are independent. Sidebar ordering persists and supports drag and drop. Ctrl+1 through Ctrl+9 follows sidebar positions. Disabling closes the WebView while retaining profile data. Enabling opens it again. Browser data can be cleared per instance.

Session persistence cannot override server-side expiry or WebKit browser restrictions imposed by external providers. Login and service-specific account functionality need the user's own accounts to verify.

## Operating Context
A native desktop wrapper for third-party web applications. Browser profiles and settings are local, under XDG directories. Test profiles must be separate from user data.

## Open Decisions
Messenger Hub is a working name based on the project directory. UI copy uses English. The user chose a compact, dark, favicon-led service rail as the durable shell direction.
