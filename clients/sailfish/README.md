# harbour-feed — Sailfish OS client

A native Sailfish OS client for [feed.](https://github.com/anothertijuano/feed),
the self-hosted ranked feed reader. Pure QML (no C++): feed browsing with
infinite scroll, upvote/downvote/save, subscriptions with per-feed notify
policy, saved list, pull-to-refresh, and an app cover.

## Features

- **Feed** — ranked cards with source, title, thumbnail and snippet;
  loads more as you scroll; pull-down **Refresh** (`POST /api/refresh`);
  items are marked as seen while you browse.
- **Vote / save** — ▲ upvote (trains the recommendation model), ▼
  downvote (removes the item from the system — confirmed with a
  Sailfish *Remorse* undo window), ★ save (mirrors to your Memos).
- **Item view** — full paragraphs, images, "Open original" in the
  browser, vote/save.
- **Saved** — everything you bookmarked, newest first.
- **Subscriptions** — add RSS/Atom/JSON feeds, remove, and set the push
  notification policy per feed (`always` / `default` / `never`).
- **Settings** — server URL + access token (persisted via
  `Nemo.Configuration`), test connection.

## Prerequisites

- The [Sailfish OS SDK](https://docs.sailfishos.org/Tools/Sailfish_SDK/)
  with a matching build target (e.g. `SailfishOS-5.x-aarch64`).
- On the phone: **Settings → Developer tools → Remote connection** with a
  password, so you can deploy over SSH.

## Build & deploy

```sh
# inside this directory (clients/sailfish)
sfdk config target=SailfishOS-latest-aarch64
sfdk build
sfdk deploy --device 10.0.1.102
```

`sfdk deploy` asks for the developer-mode password. The app appears as
**feed.** in the launcher. On first launch open the app's Settings and
enter your server URL and an access token (create one in the web app:
*Settings → Access tokens*, or with `feed -gen-token <name> -data <dir>`
on the server).

## Quick test without the SDK (on-device)

The app is pure QML, so you can run it directly on the phone without
building an RPM:

```sh
scp -r qml defaultuser@10.0.1.102:/home/defaultuser/feed-qml
ssh defaultuser@10.0.1.102
# on the device:
sailfish-qml /home/defaultuser/feed-qml/qml/harbour-feed.qml
```

## Manual install (no SDK)

You can install the app onto the device without an RPM (the phone keeps
it across reboots, but a system update may wipe it). The launcher must be
a real ELF binary — a shell script works when run by hand but the
lipstick launcher does not start it:

```sh
# from clients/sailfish, with developer-mode SSH to the phone:
cd launcher
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o harbour-feed .
cd ..
scp -r qml icons harbour-feed.desktop launcher/harbour-feed \
    defaultuser@10.0.1.102:/home/defaultuser/feed-qml/
ssh defaultuser@10.0.1.102
# on the device:
echo '<dev-mode-password>' | devel-su sh -c '
  rm -rf /usr/share/harbour-feed
  mkdir -p /usr/share/harbour-feed
  cp -r /home/defaultuser/feed-qml/qml /usr/share/harbour-feed/qml
  cp /home/defaultuser/feed-qml/harbour-feed /usr/bin/harbour-feed
  chmod 0755 /usr/bin/harbour-feed
  cp /home/defaultuser/feed-qml/harbour-feed.desktop /usr/share/applications/harbour-feed.desktop
  for s in 86x86 108x108 128x128 172x172 256x256; do
    mkdir -p /usr/share/icons/hicolor/$s/apps
    cp /home/defaultuser/feed-qml/icons/$s.png /usr/share/icons/hicolor/$s/apps/harbour-feed.png
  done'
systemctl --user restart lipstick   # pick up the new launcher entry
```

The launcher appends its log to `/tmp/harbour-feed.log` on the phone.

## Layout

```
qml/harbour-feed.qml      ApplicationWindow, settings, page wiring
qml/js/api.js             shared XMLHttpRequest API helper (pragma library)
qml/pages/FeedPage.qml    ranked feed
qml/pages/ItemPage.qml    full article
qml/pages/SavedPage.qml   saved items
qml/pages/SubsPage.qml    subscription management
qml/pages/SettingsPage.qml  server URL + token
qml/cover/CoverPage.qml   app cover
rpm/                      packaging (spec + yaml)
icons/                    86–256 px app icons
```
