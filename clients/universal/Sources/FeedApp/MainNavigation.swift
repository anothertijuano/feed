import SwiftUI
#if canImport(FeedCore)
import FeedCore
#endif

/// The signed-in shell: a NavigationSplitView with a sidebar on macOS, and a
/// TabView on iOS.
#if os(macOS)
@MainActor
struct MainNavigation: View {
    let client: FeedClient

    @State private var selection: Section? = .feed

    enum Section: String, CaseIterable, Identifiable {
        case feed = "Feed"
        case saved = "Saved"
        case subscriptions = "Subscriptions"
        case settings = "Settings"

        var id: String { rawValue }

        var icon: String {
            switch self {
            case .feed: "newspaper"
            case .saved: "bookmark"
            case .subscriptions: "dot.radiowaves.left.and.right"
            case .settings: "gearshape"
            }
        }
    }

    var body: some View {
        NavigationSplitView {
            List(selection: $selection) {
                ForEach(Section.allCases) { section in
                    Label(section.rawValue, systemImage: section.icon)
                        .tag(section)
                }
            }
            .navigationSplitViewColumnWidth(min: 170, ideal: 200)
        } detail: {
            switch selection ?? .feed {
            case .feed: FeedView(client: client)
            case .saved: SavedView(client: client)
            case .subscriptions: SubscriptionsView(client: client)
            case .settings: SettingsView(client: client)
            }
        }
    }
}
#else
@MainActor
struct MainTabView: View {
    let client: FeedClient

    var body: some View {
        TabView {
            FeedView(client: client)
                .tabItem { Label("Feed", systemImage: "newspaper") }
            SavedView(client: client)
                .tabItem { Label("Saved", systemImage: "bookmark") }
            SubscriptionsView(client: client)
                .tabItem { Label("Subscriptions", systemImage: "dot.radiowaves.left.and.right") }
            SettingsView(client: client)
                .tabItem { Label("Settings", systemImage: "gearshape") }
        }
    }
}
#endif
