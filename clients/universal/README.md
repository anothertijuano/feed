# Feed — native clients for feed2

A single SwiftUI codebase that runs as a **macOS app** (via Swift Package Manager) and an **iOS app** (via an Xcode project), sharing all source files. Both talk to a self-hosted [feed2](https://github.com/microbito/feed2) server over its JSON API.

## Layout

```
clients/universal/
├── Package.swift            # SwiftPM manifest: FeedCore library + FeedApp executable
├── Sources/
│   ├── FeedCore/            # Shared library (no UI dependencies)
│   │   ├── Models.swift           # Codable API models (FeedItem, Subscription, …)
│   │   ├── FeedClient.swift       # async/await API client (actor)
│   │   ├── FeedError.swift        # typed errors incl. server {"error": …}
│   │   ├── KeychainTokenStore.swift  # Keychain token storage w/ UserDefaults fallback
│   │   └── RFC3339.swift          # timestamp parsing
│   └── FeedApp/             # SwiftUI app (macOS + iOS)
│       ├── FeedApp.swift          # @main App
│       ├── AppState.swift         # @MainActor @Observable session (URL + token + client)
│       ├── RootView.swift         # signed-in vs connect screen
│       ├── MainNavigation.swift   # macOS NavigationSplitView / iOS TabView
│       ├── Platform.swift         # openURL, Image(data:), card colors, .hint
│       ├── ViewModels/            # @MainActor @Observable view models
│       └── Views/                 # Feed, Saved, Subscriptions, Settings, Connect, cards
└── ios/
    ├── Feed.xcodeproj/      # Xcode 16 project (iOS app target "Feed")
    └── README-build-ios.md  # iOS build instructions
```

## Features

- **Feed** — Instagram-like vertical cards: title, source, relative time, paragraph excerpt, optional async image (URLSession-backed cache, no external dependencies), vote up/down/clear and save with optimistic updates, pull-to-refresh, infinite scroll, "seen" reporting.
- **Saved** — everything you bookmarked, newest save first; un-save removes it.
- **Subscriptions** — list feeds, add a feed by URL, delete, per-feed push-notification policy (`Default`/`Always`/`Never`), and a "refresh all feeds now" action.
- **Settings** — server URL + access token (sign in / sign out) and the Memos mirror (URL + token). Sign-in validates the credentials before persisting them.
- **Muted colors, dark mode** via system colors; no third-party packages.

## macOS

Requirements: Xcode Command Line Tools with Swift 6 (or a full Xcode install).

```sh
cd clients/universal
swift build          # builds FeedCore and FeedApp
swift run FeedApp    # launches the app
```

The first run shows a **Connect** screen: enter your server URL (e.g. `https://feed.example.com`) and an access token (`ft_…`, created in the web UI under *Settings → Access tokens*). The token is stored in the Keychain (with a UserDefaults fallback for unsigned CLI builds); the server URL in UserDefaults. Sign out removes both.

> Note: the default toolchain path on this machine is
> `/Library/Developer/CommandLineTools/usr/bin/swift`. If `swift build`
> complains it cannot load the macOS standard library, pass the SDK
> explicitly:
> `SDKROOT=$(xcrun --sdk macosx --show-sdk-path) swift build`.

## iOS

Open `ios/Feed.xcodeproj` in **Xcode 16 or newer**, select the *Feed* scheme and a simulator or device, and run. See `ios/README-build-ios.md` for signing and caveats.

## How the shared code works

- `Sources/` is the single source of truth. The SwiftPM `FeedApp` target depends on the `FeedCore` library and imports it with `#if canImport(FeedCore) … import FeedCore`.
- The Xcode project compiles **all** of `Sources/` into the single iOS app target via a `PBXFileSystemSynchronizedRootGroup` pointing at `../Sources`. There the `canImport(FeedCore)` check is false, so the FeedCore sources are simply part of the app module — no framework needed.
- Platform differences are handled with `#if os(macOS)` / `#if os(iOS)`:
  - macOS: `NavigationSplitView` with a sidebar; iOS: `TabView`.
  - Image decoding via `NSImage` / `UIImage`; links open via `NSWorkspace` / `UIApplication`.
- All state is `@MainActor @Observable` (iOS 17 / macOS 14), and the API client is a Swift 6 actor with typed async methods for every endpoint.

## Notes & caveats

- Downvoting an item removes it from the feed (server behavior) — the card disappears after the request succeeds.
- Media loaded over plain `http://` is blocked by App Transport Security on iOS; use `https://` feeds or add an ATS exception.
- The server returns the raw token exactly once when you create one; the app only stores the token you paste in.
