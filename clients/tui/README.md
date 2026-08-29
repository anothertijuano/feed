# feedtui

A terminal (TUI) client for [feed](https://github.com/anothertijuano/feed) —
the self-hosted feed reader. Browse your feed, vote, save, manage subscriptions
and edit settings from the comfort of your terminal.

## Install

Once released:

```sh
go install github.com/anothertijuano/feed/tui@latest
```

From source (this repository):

```sh
cd clients/tui
go build -o feedtui .
```

Requires Go 1.25+.

## Usage

```sh
feedtui [-server URL] [-token TOKEN]
```

| Option    | Meaning                                            |
| --------- | -------------------------------------------------- |
| `-server` | Feed server URL (default `http://localhost:8084`)  |
| `-token`  | API token (`Authorization: Bearer <token>`)        |

Configuration is resolved in this order (first match wins):

1. `-server` / `-token` flags
2. `FEED_SERVER` / `FEED_TOKEN` environment variables
3. `~/.config/feedtui/config.json`
4. Built-in default server `http://localhost:8084`

The config file is created automatically when you save settings from the
Settings tab (or you can create it yourself):

```json
{
  "server": "http://localhost:8084",
  "token": "your-token"
}
```

## Keymap

| Key              | Where    | Action                                         |
| ---------------- | -------- | ---------------------------------------------- |
| `tab` / `shift+tab` | all   | Next / previous tab                            |
| `1`–`4`          | all      | Jump to tab                                    |
| `?`              | all      | Help overlay                                   |
| `q` / `ctrl+c`   | all      | Quit                                           |
| `↑` `↓` / `j` `k` | lists  | Move cursor                                    |
| `u`              | lists    | Upvote                                         |
| `d`              | lists    | Downvote (**removes the item permanently**)    |
| `0`              | lists    | Clear vote                                     |
| `s`              | lists    | Toggle save                                    |
| `o` / `enter`    | lists    | Open link in the system browser                |
| `r`              | lists    | Refresh feed (POST /api/refresh) then reload / reload list |
| `a`              | Subs     | Add subscription (prompt for URL, enter confirms, esc cancels) |
| `d`              | Subs     | Delete subscription (`y` confirms, anything else cancels) |
| `n`              | Subs     | Cycle notify policy: default → always → never  |
| `e` / `enter`    | Settings | Edit a settings field                          |
| `tab` / `enter`  | Settings | Next field (enter on the last field saves)     |
| `ctrl+s`         | Settings | Save settings                                  |
| `esc`            | Settings | Cancel editing                                 |
| `r`              | Settings | Reload server settings                         |

## Tabs

- **Feed** — the ranked feed, paged 20 items at a time. Pages load
  automatically as you scroll near the bottom. Rows show the vote (▲/▼), save
  mark (★), title, source and relative age. Items are marked *seen* on the
  server the first time they are displayed.
- **Saved** — everything you saved, newest first.
- **Subs** — your subscriptions with item counts and notify policy badges
  (● always / ● never / ○ default).
- **Settings** — server URL, token and memos settings. Saving writes
  `~/.config/feedtui/config.json`, applies the new server/token immediately and
  pushes the memos settings to the server.

A braille spinner in the header shows during every network request; the footer
shows the last success/error status, including the server's `error` message
when a request fails.

## Notes

- Downvoting removes the item from the system permanently (server-side
  behaviour of `POST /api/interactions` with `vote: -1`).
- Opening links uses `open` on macOS and `xdg-open` on Linux.

## Development

```sh
go vet ./... && go test ./... && go build ./...
```

`FEED_SERVER` and `FEED_TOKEN` can be used to run a read-only smoke test
against a live server: `go test -run TestLiveServer -v`.
