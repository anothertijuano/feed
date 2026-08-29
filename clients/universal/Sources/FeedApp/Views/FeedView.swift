import SwiftUI
#if canImport(FeedCore)
import FeedCore
#endif

/// The main feed: vertical cards, pull-to-refresh, infinite scroll.
@MainActor
struct FeedView: View {
    @State private var vm: FeedViewModel

    init(client: FeedClient) {
        _vm = State(initialValue: FeedViewModel(client: client))
    }

    var body: some View {
        content
            .navigationTitle("Feed")
            .background(Color.screenBackground)
            .task { await vm.loadInitial() }
    }

    @ViewBuilder
    private var content: some View {
        if vm.items.isEmpty {
            if vm.isLoading {
                ProgressView("Loading feed…")
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
            } else if let errorMessage = vm.errorMessage {
                ContentUnavailableView {
                    Label("Couldn't Load the Feed", systemImage: "wifi.exclamationmark")
                } description: {
                    Text(errorMessage)
                } actions: {
                    Button("Try Again") {
                        Task { await vm.refresh() }
                    }
                }
            } else {
                ContentUnavailableView {
                    Label("Nothing Here Yet", systemImage: "newspaper")
                } description: {
                    Text("Add subscriptions in the Subscriptions tab, then refresh.")
                }
            }
        } else {
            feedList
        }
    }

    private var feedList: some View {
        ScrollView {
            LazyVStack(alignment: .leading, spacing: 14) {
                if let errorMessage = vm.errorMessage {
                    errorBanner(errorMessage)
                }

                ForEach(vm.items) { item in
                    FeedCardView(
                        item: item,
                        onOpen: { openLink(item) },
                        onVote: { value in
                            Task { await vm.vote(item.id, value) }
                        },
                        onToggleSaved: {
                            Task { await vm.toggleSaved(item.id) }
                        }
                    )
                    .onAppear {
                        Task { await vm.markSeen(item.id) }
                        if item.id == vm.items.last?.id {
                            Task { await vm.loadMore() }
                        }
                    }
                }

                if vm.isLoadingMore {
                    ProgressView()
                        .frame(maxWidth: .infinity)
                        .padding(.vertical, 8)
                }
            }
            .padding(16)
        }
        .refreshable { await vm.refresh() }
    }

    private func errorBanner(_ message: String) -> some View {
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
