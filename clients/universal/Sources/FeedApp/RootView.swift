import SwiftUI
#if canImport(FeedCore)
import FeedCore
#endif

/// Chooses between the signed-in interface and the connect (sign-in) screen.
@MainActor
struct RootView: View {
    @Environment(AppState.self) private var appState

    var body: some View {
        Group {
            if appState.isSignedIn, let client = appState.client {
                #if os(macOS)
                MainNavigation(client: client)
                #else
                MainTabView(client: client)
                #endif
            } else {
                ConnectView()
            }
        }
        .animation(.default, value: appState.isSignedIn)
    }
}
