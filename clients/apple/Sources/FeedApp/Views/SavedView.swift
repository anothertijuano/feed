import SwiftUI
#if canImport(FeedCore)
import FeedCore
#endif

/// All saved items, newest save first.
@MainActor
struct SavedView: View {
    @State private var vm: SavedViewModel

    init(client: FeedClient) {
        _vm = State(initialValue: SavedViewModel(client: client))
    }

    var body: some View {
        content
            .navigationTitle("Saved")
            .background(Color.screenBackground)
            .task { await vm.load() }
    }

    @ViewBuilder
    private var content: some View {
        if vm.items.isEmpty {
            if vm.isLoading {
                ProgressView("Loading saved items…")
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
            } else if let errorMessage = vm.errorMessage {
                ContentUnavailableView {
                    Label("Couldn't Load Saved Items", systemImage: "wifi.exclamationmark")
                } description: {
                    Text(errorMessage)
                } actions: {
                    Button("Try Again") {
                        Task { await vm.refresh() }
                    }
                }
            } else {
                ContentUnavailableView {
                    Label("No Saved Items", systemImage: "bookmark")
                } description: {
                    Text("Tap the bookmark on a story to keep it here.")
                }
            }
        } else {
            ScrollView {
                LazyVStack(alignment: .leading, spacing: 14) {
                    if let errorMessage = vm.errorMessage {
                        banner(errorMessage)
                    }
                    ForEach(vm.items) { item in
                        FeedCardView(
                            item: item,
                            onOpen: { openLink(item) },
                            onVote: { value in
                                Task { await vm.vote(item.id, value) }
                            },
                            onToggleSaved: {
                                Task { await vm.unsave(item.id) }
                            }
                        )
                    }
                }
                .padding(16)
            }
            .refreshable { await vm.refresh() }
        }
    }

    private func banner(_ message: String) -> some View {
        HStack(spacing: 8) {
            Image(systemName: "exclamationmark.triangle.fill")
                .foregroundStyle(.yellow)
            Text(message)
                .font(.footnote)
            Spacer()
        }
        .padding(10)
        .background(
            RoundedRectangle(cornerRadius: 10, style: .continuous)
                .fill(Color.yellow.opacity(0.12))
        )
    }

    private func openLink(_ item: FeedItem) {
        guard let url = URL(string: item.link) else { return }
        openExternalURL(url)
    }
}
