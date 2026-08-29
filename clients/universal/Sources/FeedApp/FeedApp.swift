import SwiftUI

@main
@MainActor
struct FeedApp: App {
    @State private var appState = AppState()

    var body: some Scene {
        WindowGroup {
            RootView()
                .environment(appState)
                .frame(minWidth: 480, minHeight: 560)
        }
    }
}
