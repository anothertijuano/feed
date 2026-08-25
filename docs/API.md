# API

All endpoints speak JSON. When `-htpasswd` is configured, every request
(except the public paths below) needs HTTP Basic authentication. CORS is
enabled, so the API is usable from third-party web and native clients.

## Public paths (no auth)

| Path | Purpose |
| --- | --- |
| `GET /api/health` | liveness for uptime monitors |
| `GET /manifest.json`, `GET /sw.js`, `GET /icons/*` | static PWA plumbing (no data) |

## Content

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/feed?limit=&offset=` | ranked content, annotated with vote/save state |
| `GET` | `/api/saved` | saved content, newest save first |

`GET /api/feed` response:

```json
{
  "total": 42,
  "items": [
    {
      "id": "r1a2b3c4d5e6",
      "title": "…",
      "link": "https://…",
      "sourceName": "example.com",
      "media": [{"src": "https://…", "contain": false}],
      "paragraphs": ["…"],
      "subscription": "s…",
      "fetchedAt": "2026-08-25T10:00:00Z",
      "vote": 1,
      "saved": false
    }
  ]
}
```

## Interactions

| Method | Path | Purpose |
| --- | --- | --- |
| `POST` | `/api/interactions` | record a vote or save |
| `GET` | `/api/interactions` | full interaction state |

```json
{"key": "<item id>", "kind": "vote", "value": -1 | 0 | 1}
{"key": "<item id>", "kind": "save", "value": true | false}
```

- `vote: 1` upvotes and trains the recommendation model.
- `vote: -1` removes the item from the system and blocklists its link/GUID
  forever; any mirrored Memos memo is deleted.
- `vote: 0` removes an upvote.
- `save: true` also mirrors the item to the configured Memos instance.

Response:

```json
{"key": "<item id>", "vote": 1, "saved": true}
```

## Subscriptions

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/subscriptions` | list feeds (incl. `notify` policy) |
| `POST` | `/api/subscriptions` | add an RSS/Atom/JSON feed: `{"url":"https://…"}` |
| `DELETE` | `/api/subscriptions/{id}` | remove a feed and all its content |
| `POST` | `/api/subscriptions/{id}` | set policy: `{"notify":"default"\|"always"\|"never"}` |
| `POST` | `/api/refresh` | fetch all feeds now; returns `{"new": N}` |

## Push notifications

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/push/key` | VAPID public key for `pushManager.subscribe` |
| `POST` | `/api/push/subscribe` | register a Web Push subscription |
| `DELETE` | `/api/push/unsubscribe` | `{"endpoint":"…"}` |

Subscribe body (the browser's `PushSubscription` JSON):

```json
{
  "endpoint": "https://…",
  "keys": {"p256dh": "…", "auth": "…"}
}
```

## Settings

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/settings` | Memos URL/token and sync status |
| `POST` | `/api/settings` | `{"memosUrl":"https://…","memosToken":"…"}` |

Configuring Memos backfills any previously saved items.

## Errors

Errors are JSON with a 4xx/5xx status:

```json
{"error": "human readable message"}
```
