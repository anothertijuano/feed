# Internals

Developer-facing details: on-disk database and process architecture.

## Database

Plain JSON files in the data directory (default `./data`, flag `-data`),
consumable by any utility:

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
  seen.json           — articles the user has already seen
  vapid.json          — VAPID key pair (mode 0600)
  tokens.json         — access-token hashes (mode 0600)
  items/<id>.json     — one file per content item
```

## Goroutines

1. **main** — flags, signal handling
2. **server** — HTTP: serves content, listens to client messages
3. **fetcher** — polls subscriptions (ETag/Last-Modified)
4. **extractor** — parses feeds into content items
5. **ranker** — trains the recommendation model from votes, sorts content
6. **notifier** — applies the notification policy and sends Web Push

## Ranking

Relevance = source affinity + title-keyword affinity + recency boost.

- Each upvote moves the item's source and title tokens toward +1 (rate
  0.15); each downvote toward −1.
- Downvoted content is removed and its link/GUID blocklisted forever.
- Seen/liked content sinks below fresh content — the feed always leads
  with articles you have not read yet.
- The model survives restarts (`model.json`).
- Push (default policy) fires for items scoring above `-push-threshold`.

## Repository layout

```
.
├── frontend/          ← the PWA (plain HTML/CSS/JS, no build step)
├── *.go               ← the backend (Go module at the repo root)
├── .github/workflows/ ← release pipeline
└── docs/              ← API + internals documentation
```

The frontend is embedded into the binary at build time via `go:embed`
(`main.go`) — embed patterns cannot cross module boundaries, so the Go
module lives at the repo root.
