# Building the iOS app

The iOS app shares 100% of its source with the macOS app, in `../Sources`
(`FeedCore` + `FeedApp`). There is no duplicate code — `Sources/` is the
single source of truth.

## Requirements

- Xcode 16.0 or newer (the project uses the Xcode 16 project format:
  `objectVersion = 77` with a `PBXFileSystemSynchronizedRootGroup`).
- iOS 17.0+ (deployment target is 17.0; the UI uses `@Observable`,
  `ContentUnavailableView`, etc.).

## Run it

1. Open `Feed.xcodeproj` (in this `ios/` folder) in Xcode.
2. In the toolbar, select the **Feed** scheme and a simulator (e.g. iPhone 15)
   or your device, then hit Run.
3. On first launch you get the **Connect** screen: enter your server URL
   (`https://…`) and an access token (`ft_…`, created in the web UI under
   *Settings → Access tokens*). These are validated before being persisted
   (token → Keychain, URL → UserDefaults).

## Signing

The project has `CODE_SIGN_STYLE = Automatic` and **no development team
pre-set**. Before running on a physical device:

1. Select the Feed target → *Signing & Capabilities*.
2. Pick your team (or your personal team for a free account).
3. If `com.anothertijuano.feed.ios` is taken by another team, change the
   bundle identifier.

Simulator builds don't require a team.

## How the project is set up

- One app target, **Feed** (`com.apple.product-type.application`,
  `IPHONEOS`), bundle id `com.anothertijuano.feed.ios`, product `Feed.app`.
- A single `PBXFileSystemSynchronizedRootGroup` points at `../Sources`
  (relative to the `ios/` folder). Xcode auto-discovers every Swift file
  under `Sources/` — add new files there and they appear in the app
  automatically, no project edits needed.
- Both `FeedCore` and `FeedApp` sources compile into the same module
  (`Feed`, the app's module). FeedApp files import the core types via
  `#if canImport(FeedCore) import FeedCore #endif` — true when built by
  SwiftPM on macOS, false here, where the core types are already in the
  same module.
- `GENERATE_INFOPLIST_FILE = YES`: the Info.plist is generated from the
  `INFOPLIST_KEY_*` build settings (SwiftUI scene manifest, launch screen,
  supported orientations). No hand-written Info.plist.

## Caveats

- There is no asset catalog or app icon yet; the app shows the default
  blank icon. Add an `Assets.xcassets` with an `AppIcon` (and set
  `ASSETCATALOG_COMPILER_APPICON_NAME = AppIcon`) when you want one.
- Media served over plain `http://` is blocked by App Transport Security.
  Use `https://` feeds, or add `NSAppTransportSecurity` exceptions to the
  generated Info.plist via build settings.
- Push notifications are *not* implemented natively (no `UNUserNotification`
  wiring); the per-feed notify policies still apply to the web push
  subscribers and are editable from the Subscriptions tab.
