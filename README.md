# feed.

A self-hosted, mobile-first feed reader. Subscribe to RSS/Atom/JSON feeds,
get articles ranked by a model that learns from your votes, and receive
push notifications for noteworthy content. Installable as a PWA; the server
is a single static binary.

## Features

- 📡 RSS, Atom and JSON Feed subscriptions, polled automatically
- 🎯 Relevance ranking that learns from your up/down votes (downvotes
  remove the article for good)
- 🔔 Push notifications for high-rank articles, with per-source rules
  (always / never / by rank)
- 🔖 Saved content, optionally mirrored to a [Memos](https://www.usememos.com/) instance
- 📱 Installable PWA with offline app shell
- 🔐 Basic auth (htpasswd) for self-hosting behind your gateway

## Install

### From GitHub releases

1. Grab the binary for your platform from the
   [latest release](https://github.com/anothertijuano/feed/releases/latest):

   | File | Platform |
   | --- | --- |
   | `feed-linux-amd64` | Linux x86-64 |
   | `feed-darwin-amd64` | macOS (Intel; works on Apple Silicon via Rosetta) |

2. Install it:

   ```sh
   sudo install -m 0755 feed-linux-amd64 /usr/local/bin/feed
   ```

   *(On Apple Silicon you can also build natively — see “From source”.)*

3. Verify:

   ```sh
   feed -gen-vapid    # should print a VAPID key pair
   ```

### With Go

```sh
go install github.com/anothertijuano/feed@latest
```

Requires Go 1.25+.

### From source

```sh
git clone git@github.com:anothertijuano/feed.git
cd feed
make build          # ./feed
sudo make install   # installs to /usr/local/bin/feed
```

## Quick start

```sh
feed -addr :8000 -data ./data
```

Open `http://localhost:8000`, go to the **Subs** tab, paste a feed URL and
hit Add. New articles appear ranked by relevance; vote on them to teach the
ranking. Pull down at the top of the feed (or tap ↻) to refresh.

| Tab | What it does |
| --- | --- |
| Feed | ranked infinite scroll; ♥/↓/bookmark to vote and save, double-tap to like |
| Saved | everything you bookmarked |
| Subs | add/remove feeds, refresh, per-source notification rules (bell) |
| Settings | Memos connection, push notifications |

## Configuration

| Flag | Default | Meaning |
| --- | --- | --- |
| `-addr` | `:8000` | address to listen on |
| `-data` | `data` | directory for the on-disk database |
| `-refresh` | `15m` | how often subscriptions are polled |
| `-htpasswd` | *(empty)* | htpasswd file → enables HTTP Basic auth |
| `-push-threshold` | `0.3` | rank threshold for push notifications |
| `-notify-age` | `48h` | max article age eligible for notifications |
| `-max-per-source` | `4` | max articles from one source in the diversified top of the feed |
| `-max-presents` | `3` | presentations without a reaction before an article is ignored |
| `-max-age` | `120h` | time in the feed before an article is ignored |
| `-vapid-subject` | `mailto:admin@localhost` | VAPID subject (your email/URL) |
| `-gen-vapid` | *(flag)* | print a VAPID key pair and exit |
| `-gen-token` | *(flag)* | create an access token with the given name, print it once, and exit (requires `-data`) |

## Security & sign-in

feed. works like Memos: **each client signs in with its own access token**
(`Authorization: Bearer …`). The UI shell is public and shows a sign-in
screen, so an installed PWA can always re-authenticate — no browser
credential prompts (which iOS PWAs cannot re-trigger).

- Create tokens in **Settings → Access tokens** (shown once), or on the
  server with `feed -gen-token "my iphone" -data <dir>`. The token is
  written to `<dir>/tokens.json`, which must be the **same data directory
  the service runs with** — `-gen-token` refuses to run without `-data`
  to avoid writing a token somewhere the service never reads.
- Tokens are stored as SHA-256 hashes (`data/tokens.json`, mode 0600) and
  can be revoked per client.
- `-htpasswd` is still supported as an additional way in (Basic auth), for
  gateways and third-party clients.
- Until the first token is created, the API is open (convenient for first
  setup); once a token has ever been created, authentication stays
  enforced even if all tokens are later revoked.
- The service speaks plain HTTP — TLS is the gateway's job. **Web Push
  requires HTTPS in the browser**, so the gateway must terminate TLS.
- VAPID keys are generated into `data/vapid.json` (0600) on first start.
  Set `-vapid-subject` to an email you control.

## Notifications

High-rank articles notify you automatically. In the **Subs** tab, each
source has a bell: *default* (notify on high rank), *always* (every new
article), *never*. Enable push in **Settings → Notifications** — on iOS,
add the app to the home screen first.

## Updating

Download the new binary from the releases page and replace the old one.
Your data (votes, subscriptions, saved items) lives in the `-data`
directory and is preserved across upgrades.

## Run as a macOS service

A LaunchAgent template lives in [`contrib/`](contrib/). Edit the paths
(launchd does not expand `~`), then:

```sh
cp contrib/com.anothertijuano.feed.plist ~/Library/LaunchAgents/
launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/com.anothertijuano.feed.plist
```

The agent starts at login and restarts automatically (`KeepAlive`). Logs go
to `~/Library/Logs/com.anothertijuano.feed.log`. Useful commands:

```sh
launchctl kickstart -k gui/$(id -u)/com.anothertijuano.feed   # restart
launchctl bootout    gui/$(id -u)/com.anothertijuano.feed     # stop + unload
```

Notes: keep the log paths on the internal disk (launchd refuses to spawn
jobs whose logs live on an external volume), and make sure the `-data`
directory is on an always-mounted volume. macOS also gates access to
removable volumes per binary (TCC): a launchd job opening files on an
external drive can block or be denied silently — for a server, keep both
`-data` and `-htpasswd` on the internal disk. When upgrading the binary,
replace it via `rm` + copy (or temp file + `mv`), never by overwriting it
in place while the service is running — and run it once manually (e.g.
`feed -gen-vapid`) before restarting the service, so Gatekeeper's
first-launch assessment doesn't kill the launchd-spawned process. To run
before login instead, install the plist in `/Library/LaunchDaemons/` with
a `UserName` key.

## For developers

- [API reference](docs/API.md) — endpoints for third-party clients
- [Internals](docs/INTERNALS.md) — database layout and architecture

```sh
make test    # run the test suite
make run     # serve on :8000 with data in ./data
```

Frontend assets are embedded at build time — restart to pick up changes.
