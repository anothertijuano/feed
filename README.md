# feed.

A self-hosted, mobile-first feed reader. Subscribe to RSS/Atom/JSON feeds,
get content ranked by a recommendation model that learns from your votes,
and receive push notifications for noteworthy articles. Installable as a
PWA; the backend is a single standalone binary.

## Build

Requires Go 1.25+.

```sh
make build          # ./feed (static binary, frontend embedded)
make test           # run the test suite
```

## Run

```sh
./feed -addr :8000 -data ./data
```

| Flag               | Default                     | Meaning                                             |
| ------------------ | --------------------------- | --------------------------------------------------- |
| `-addr`            | `:8000`                     | address to listen on                                |
| `-data`            | `data`                      | directory for the on-disk database                  |
| `-refresh`         | `15m`                       | how often subscriptions are polled                  |
| `-htpasswd`        | *(empty = no auth)*         | path to an htpasswd file → enables HTTP Basic auth  |
| `-push-threshold`  | `0.3`                       | rank threshold for push notifications (default policy) |
| `-notify-age`      | `48h`                       | max item age eligible for notifications             |
| `-vapid-subject`   | `mailto:admin@localhost`    | VAPID JWT subject (an email or URL you control)     |
| `-gen-vapid`       | *(flag)*                    | print a VAPID key pair and exit                     |

## Security

The app is meant to be exposed on a network, so treat auth seriously:

```sh
# create an htpasswd file (bcrypt)
htpasswd -Bc htpasswd admin

# run with auth enabled
./feed -addr :8000 -htpasswd htpasswd
```

- The htpasswd file supports bcrypt (`htpasswd -B`), `{SHA}` and plaintext
  entries, and is hot-reloaded when it changes.
- Everything is protected except `GET /api/health` (for uptime monitors)
  and the static PWA plumbing (`/manifest.json`, `/sw.js`, `/icons/*`),
  which contains no data and must be fetchable outside the authenticated
  page context.
- **TLS is the gateway's job**: the service speaks plain HTTP and is
expected to sit behind a reverse proxy (Caddy, nginx, Traefik) that
does TLS termination and forwards to `-addr`. Web Push only works in the
browser over HTTPS, so the gateway must terminate TLS.
- VAPID keys are auto-generated into `data/vapid.json` on first start
  (mode 0600). They can be regenerated or inspected with `-gen-vapid`.
- Third-party clients authenticate with the same Basic credentials.

## PWA

Open the app in Safari or Firefox on mobile → Share → *Add to Home Screen*
(it's installable via the manifest + service worker). The app-shell is
cached for offline use. Web Push requires a secure context: HTTPS, or
`localhost` for development. iOS requires the app to be added to the home
screen before notifications work.

## Push notifications

The **notifier** goroutine watches freshly ingested articles:

- Articles ranked above `-push-threshold` (source + keyword affinity,
  learned from your votes) trigger a notification.
- Per-source policy in the Subs tab (bell button):
  - **default** — notify on high rank,
  - **always** — notify for every new article,
  - **never** — never notify.
- Articles older than `-notify-age` and already-notified articles are
  skipped (no re-notifications, no first-fetch floods).
- Subscriptions are managed in Settings → Notifications; dead push
  endpoints are pruned automatically.

## API

All JSON. Authenticate with HTTP Basic when `-htpasswd` is set. CORS is
enabled, so the API is usable from third-party web clients too.

| Method   | Path                          | Purpose                                        |
| -------- | ----------------------------- | ---------------------------------------------- |
| `GET`    | `/api/health`                 | liveness (no auth required)                    |
| `GET`    | `/api/feed?limit=&offset=`    | ranked content, annotated with vote/save state |
| `GET`    | `/api/saved`                  | saved content, newest first                    |
| `POST`   | `/api/interactions`           | `{"key","kind":"vote","value":-1\|0\|1}` or `{"kind":"save","value":bool}` |
| `GET`    | `/api/interactions`           | full interaction state                         |
| `GET`/`POST` | `/api/settings`            | Memos URL + access token                       |
| `GET`    | `/api/subscriptions`          | list subscriptions (incl. `notify` policy)     |
| `POST`   | `/api/subscriptions`          | add an RSS/Atom/JSON feed URL                  |
| `DELETE` | `/api/subscriptions/{id}`     | remove a feed and its content                  |
| `POST`   | `/api/subscriptions/{id}`     | update policy: `{"notify":"default\|always\|never"}` |
| `POST`   | `/api/refresh`                | fetch all feeds now; returns `{"new": N}`      |
| `GET`    | `/api/push/key`               | VAPID public key for `pushManager.subscribe`   |
| `POST`   | `/api/push/subscribe`         | register a Web Push subscription               |
| `DELETE` | `/api/push/unsubscribe`       | `{"endpoint":"…"}`                             |

### Memos integration

Saving content stores it locally and mirrors it to a Memos instance
(`POST /api/v1/memos`, visibility `PRIVATE`); unsaving deletes the memo.
Existing saves are backfilled when you configure Memos. The sync status is
shown in Settings. The access token is stored in plain text in
`data/settings.json`.

## Database

Plain JSON files in `data/`, consumable by any utility:

```
data/
  feed.json           — votes, saves (+ save timestamps, memo names)
  settings.json       — Memos URL/token and sync status
  subscriptions.json  — subscribed feeds (+ fetch state, notify policy)
  blocked.json        — links/GUIDs that must never be fetched again
  model.json          — learned relevance weights (sources, title tokens)
  rank.json           — current relevance ordering
  push.json           — Web Push subscriptions
  notified.json       — already-notified article IDs
  vapid.json          — VAPID key pair (mode 0600)
  items/<id>.json     — one file per content item
```

## Repository layout

```
.
├── frontend/          ← the PWA (plain HTML/CSS/JS, no build step)
│   ├── index.html, styles.css, app.js
│   ├── sw.js, manifest.json
│   └── icons/
├── *.go               ← the backend (Go module at the repo root)
├── Makefile
└── README.md
```

The frontend is embedded into the binary at build time via `go:embed`
(`main.go`), so the Go module has to live at the repo root — embed
patterns cannot cross module boundaries.

## Architecture

Six goroutines:

1. **main** — flags, signal handling
2. **server** — HTTP: serves content, listens to client messages
3. **fetcher** — polls subscriptions (ETag/Last-Modified)
4. **extractor** — parses feeds into content items
5. **ranker** — trains the recommendation model from votes, sorts content
6. **notifier** — applies the notification policy and sends Web Push

Relevance = source affinity + title-keyword affinity + recency boost.
Upvotes move an item's source and tokens toward +1, downvotes toward −1;
downvoted content is removed and its link/GUID blocklisted forever.

## Development

```sh
make run            # serves on :8000, data in ./data
```

Frontend assets are embedded at build time — restart to pick up changes.
