# feed-gtk4

Native GTK4 / libadwaita client for the self-hosted feed reader.

## Features

- `AdwNavigationSplitView` with Feed / Saved / Subscriptions / Settings sections
- Feed: card list (source + time, title, snippet, thumbnail) with infinite
  scrolling; upvote / downvote / clear with a destructive downvote
  confirmation; save toggle; "seen" interaction sent on first render
- Refresh button in the header bar that `POST /api/refresh`s and reloads
- Saved: same card list without vote controls; un-saving removes the row
- Subscriptions: add dialog, remove with confirmation, notification policy
  cycling (default / always / never) and a refresh-all button
- Settings: server URL, API token (password entry), connection test,
  memos URL/token and a save button
- All network activity is asynchronous (libsoup); a busy spinner shows in the
  header bar while requests run and errors surface as toasts
- Dark mode works automatically via libadwaita; no hardcoded colors

## Build dependencies

Debian/Ubuntu:

```sh
sudo apt install libgtk-4-dev libadwaita-1-dev libsoup-3.0-dev libjson-glib-dev meson ninja-build
```

## Build

```sh
meson setup build
ninja -C build
```

## Run

```sh
build/src/feed-gtk4
```

## Install

```sh
meson install -C build
```

This installs the `feed-gtk4` binary, the desktop file and the application
icon.

## Configuration

The server URL and API token are configured from the Settings view and stored
in `~/.config/feed/gtk4.conf` (a GKeyFile, written with permissions `0600`).
There is no built-in server address: the first time you run the app, open
Settings, enter your server URL and token, and press Save.
