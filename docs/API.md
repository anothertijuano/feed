# API

All endpoints speak JSON. Authentication: send an access token as
`Authorization: Bearer <token>` (tokens are created in the app's Settings
or with `feed -gen-token <name>`). When `-htpasswd` is configured, HTTP
Basic credentials are accepted as well. CORS is enabled, so the API is
usable from third-party web and native clients.

## Public paths (no auth)

| Path | Purpose |
| --- | --- |
| `/`, `/index.html`, `/styles.css`, `/app.js`, `/sw.js`, `/manifest.json`, `/icons/*` | the UI shell (no data) — always public so the PWA can show its sign-in screen |
| `GET /api/health` | liveness for uptime monitors |

Note: until the first access token has ever been created (and with no
htpasswd configured), the API itself is open.

## Access tokens

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/tokens` | list tokens (names, ids, creation times — never the token itself) |
| `POST` | `/api/tokens` | `{"name":"iPhone"}` → returns `{"token":"ft_…", …}` (the token is shown only this once) |
| `DELETE` | `/api/tokens/{id}` | revoke a token |

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
{"key": "<item id>", "kind": "seen", "value": true}
```

- `vote: 1` upvotes and trains the recommendation model.
- `vote: -1` removes the item from the system and blocklists its link/GUID
  forever; any mirrored Memos memo is deleted.
- `vote: 0` removes an upvote.
- `save: true` also mirrors the item to the configured Memos instance.
- `seen: true` marks an article as read; seen/liked content sinks below
  fresh content in the ranking.

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
