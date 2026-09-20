---
name: Messenger Hub
description: A quiet, favicon-led native shell where each web service owns the stage.
colors:
  primary-violet: "#8b6cff"
  shell-charcoal: "#1f2126"
  rail-charcoal: "#24262b"
  rail-border: "#111216"
  rail-hover: "#2d3037"
  rail-selected: "#343740"
  rail-text: "#f5f6f8"
  rail-muted: "#d7d9df"
  state-muted: "#b7bac3"
  white: "#ffffff"
  toast-overlay: "rgba(20, 21, 25, 0.92)"
typography:
  title:
    fontFamily: "system-ui, sans-serif"
    fontWeight: 700
  body:
    fontFamily: "system-ui, sans-serif"
    fontWeight: 400
  label:
    fontFamily: "system-ui, sans-serif"
    fontWeight: 400
rounded:
  none: "0px"
  menu: "7px"
  toast: "8px"
  action: "10px"
spacing:
  tight: "2px"
  xs: "6px"
  sm: "8px"
  md: "12px"
  lg: "18px"
  empty: "36px"
components:
  service-rail:
    backgroundColor: "{colors.rail-charcoal}"
    width: "68px"
    padding: "8px 0"
  service-row:
    backgroundColor: "transparent"
    height: "54px"
    padding: "5px 8px 5px 7px"
  service-row-hover:
    backgroundColor: "{colors.rail-hover}"
    height: "54px"
    padding: "5px 8px 5px 7px"
  service-row-selected:
    backgroundColor: "{colors.rail-selected}"
    height: "54px"
    padding: "5px 8px 5px 7px"
  service-icon:
    size: "32px"
  add-action:
    backgroundColor: "transparent"
    textColor: "{colors.rail-muted}"
    rounded: "{rounded.action}"
    size: "42px"
  service-menu-action:
    rounded: "{rounded.menu}"
    padding: "7px 12px"
    width: "150px"
  status-toast:
    backgroundColor: "{colors.toast-overlay}"
    textColor: "{colors.white}"
    rounded: "{rounded.toast}"
    padding: "7px 12px"
  state-surface:
    backgroundColor: "{colors.shell-charcoal}"
    textColor: "{colors.rail-text}"
    padding: "36px"
---

# Design System: Messenger Hub

## Overview

**Creative North Star: "The Quiet Service Dock"**

Messenger Hub is a compact native workspace with the visual economy of a pinned desktop dock. A narrow charcoal rail carries recognizable service identities while the active website occupies every remaining pixel. Switching should take one glance; the shell should disappear from attention once the user starts working.

The shell has one deliberate custom visual layer: the dark favicon rail and its restrained violet selection edge. GTK continues to own typography, focus treatment, menus, forms, confirmations, permissions, and destructive semantics. This pairing makes the shell distinct without making the application feel foreign to Linux.

**Key Characteristics:**

- A fixed 68px charcoal rail beside a flexible, edge-to-edge web surface.
- Real cached site favicons at 32px, with service identity expressed visually instead of through permanent text.
- A darker selected row marked by a single 1px violet edge.
- One bottom Add action; service lifecycle and management actions stay in the service menu.
- Native GTK dialogs, controls, focus behavior, and English interface copy.
- Persistent, range-checked window size and maximize state on every backend, with visible-monitor position restoration on X11.

## Colors

The shell is an almost-neutral charcoal stack with one cool violet signal. Website content and site favicons provide the color; the application chrome remains quiet.

### Primary

- **Selection Violet** (`primary-violet`): Used only for the selected row's 1px left edge and other equally small focus-signaling details.

### Neutral

- **Shell Charcoal** (`shell-charcoal`): The deepest application backdrop beneath the rail and web surface.
- **Rail Charcoal** (`rail-charcoal`): The persistent service rail.
- **Rail Border** (`rail-border`): The 1px divider between the rail and the embedded service.
- **Rail Hover** (`rail-hover`): The quiet pointer-hover layer for service rows.
- **Rail Selected** (`rail-selected`): The darker selected-row fill.
- **Rail Text** (`rail-text`): High-emphasis foreground on dark shell surfaces.
- **Rail Muted** (`rail-muted`): The resting Add icon and other low-emphasis rail glyphs.
- **State Muted** (`state-muted`): Secondary explanations on empty, disabled, and clearing surfaces.
- **White** (`white`): Active rail-action and toast foreground.
- **Toast Overlay** (`toast-overlay`): Transient loading, error, and completion feedback over the active service.

**The Violet Edge Rule.** Violet is a locator, not a fill: keep it to the 1px selected edge so the active service remains obvious without turning the rail into a colored panel.

**The Site Owns Color Rule.** Preserve each service's real favicon and let the embedded website carry its own palette; do not tint or homogenize service identity.

## Typography

**Display Font:** Installed system UI font
**Body Font:** Installed system UI font
**Label Font:** Installed system UI font

**Character:** Familiar Linux desktop typography. The favicon-only rail carries no visible service labels; GTK supplies type hierarchy where language is needed in menus, state pages, toasts, and dialogs.

### Hierarchy

- **Title:** Native GTK title treatment at bold weight for empty, disabled, and clearing states.
- **Body:** Native GTK body treatment for explanations, fields, and dialog controls.
- **Label:** Native GTK control and metadata treatment for menu actions, field labels, and transient status.

**The Website Leads Rule.** Shell typography explains state or requests action; it never restates the current page title or URL above the embedded service.

## Layout

The window is a two-part composition. A fixed 68px rail spans the full height, and the embedded service expands across every remaining pixel. There is no browser-navigation row, page-title strip, URL display, or persistent Manage toolbar between the rail and the website.

Each service occupies a 54px row with a centered 32px favicon. Rows stack from the top in the user's saved order. The Add action is a 42px square target anchored after the expanding service list at the bottom of the rail. The rail keeps this geometry at the 1100×720 default window and the 800×600 minimum window; only the web surface flexes.

Dragging reorders services, and Ctrl+1 through Ctrl+9 follows the saved rail positions. Disabled services retain their positions so keyboard locations stay predictable.

The application persists the last unmaximized width and height, plus whether the window was maximized, and restores those on every GTK backend. It also stores X11 screen coordinates. Those coordinates restore only when the saved window rectangle intersects a current monitor; a stale offscreen position falls back to normal window-manager placement. On Wayland, the compositor owns placement, so the saved position is intentionally not applied. Stored sizes and coordinates are range-checked before conversion to native 32-bit geometry values.

**The Service Owns the Stage Rule.** After the 68px rail, give all available width and height to WebKit.

**The Visible Restore Rule.** Restore range-checked X11 coordinates only for a saved window rectangle that intersects a current monitor; otherwise let the window manager place the window. On Wayland, never apply saved coordinates because the compositor owns placement.

## Elevation & Depth

The shell is flat and uses tonal steps instead of shadows: rail, hover, and selected fills become progressively lighter against the charcoal base. A 1px divider separates shell from website, and the transient status toast uses an opaque dark overlay. Popovers and dialogs use GTK's native elevation.

**The Flat Rail Rule.** Do not add shadows, gloss, gradients, or floating containers to the rail; state comes from tone, opacity, and the violet edge.

## Shapes

Service rows remain rectangular so they read as one continuous dock. The Add action uses a gently rounded 10px target, menu actions use 7px corners, and transient toasts use 8px corners. Favicons keep their source artwork and intrinsic silhouette. GTK-native dialogs and form controls keep the active desktop theme's shapes.

## Components

### Service rail

The 68px full-height rail is the durable shell. It uses Rail Charcoal, a 1px Rail Border on the web-facing edge, 8px vertical padding, and no horizontal padding. The scrollable service list occupies the flexible height; the Add action is the single fixed item below it.

### Service row

Each 54px row centers one real cached 32px site favicon. Hover shifts to Rail Hover. Selection shifts to Rail Selected and adds the 1px Selection Violet edge. A disabled service stays in place and reduces only its favicon opacity to 38%.

Favicon-only rows expose the service name through the accessibility label and tooltip. Disabled rows append “disabled” to that accessible name. A generic browser symbol appears only while no favicon is cached.

### Add action

The bottom Add action is a 42px square icon button with a 10px radius, transparent resting background, muted glyph, and selected-color hover surface. Its tooltip reads “Add service (Ctrl+N)”. Do not add permanent text or additional utility icons around it.

### Service menu

Right-clicking a service opens a compact GTK popover beside its row. Menu or Shift+F10 opens the same menu for the focused selected row. The ordered actions are Disable or Enable, Refresh, Home, and Manage; unavailable actions remain insensitive while the service is disabled or busy.

### Embedded service

The WebKit surface begins immediately after the rail and fills the rest of the window. Service pages retain their own typography, colors, navigation, and account UI. The shell does not repeat browser Back, Forward, Reload, Home, title, URL, or Manage controls above the page; Refresh, Home, and Manage live in the service menu, with Ctrl+R retained for refresh.

### Status and state surfaces

Loading, errors, and completion messages appear in a compact dark toast centered near the bottom of the web surface. Empty, disabled, and clearing states use Shell Charcoal with Rail Text for titles and State Muted for explanations, centered around the next relevant action with 36px padding. Add, Manage, permission, clear-data, and removal dialogs remain native GTK dialogs with English labels and semantic destructive treatment. Manage keeps service lifecycle and the per-service **Desktop notifications** preference together as native checkboxes.

## Do's and Don'ts

### Do:

- **Do** preserve the 68px rail, 54px rows, and 32px cached site favicons.
- **Do** identify the selected service with the darker row and a single 1px violet edge.
- **Do** expose each favicon-only row's service name and disabled state to assistive technology.
- **Do** make Disable or Enable, Refresh, Home, and Manage available from right-click, Menu, and Shift+F10.
- **Do** keep shell copy in English and keep dialogs, focus behavior, and destructive semantics native to GTK.
- **Do** reserve the bottom of the rail for the single Add icon.

### Don't:

- **Don't** expand the compact rail into a permanent text sidebar or add visible service names beside the favicons.
- **Don't** restore the browser navigation, page-title, URL, or visible Manage toolbar.
- **Don't** replace a cached site favicon with a generic or recolored application glyph.
- **Don't** add extra bottom-rail controls, decorative shadows, gradients, or broad violet fills.
- **Don't** style native dialogs as custom web panels.
