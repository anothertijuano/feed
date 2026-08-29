import Foundation
import Observation
#if canImport(FeedCore)
import FeedCore
#endif

/// State and actions for the main feed: pagination, pull-to-refresh and
/// optimistic vote/save interactions.
@MainActor
@Observable
public final class FeedViewModel {
    public private(set) var items: [FeedItem] = []
    public private(set) var isLoading = false
    public private(set) var isLoadingMore = false
    public private(set) var errorMessage: String?

    private let client: FeedClient
    private let pageSize: Int
    private var total = 0
    private var hasLoaded = false
    private var seenKeys: Set<String> = []

    public init(client: FeedClient, pageSize: Int = 30) {
        self.client = client
        self.pageSize = pageSize
    }

    public var hasMore: Bool { items.count < total }

    /// Loads the first page once; subsequent calls are no-ops.
    public func loadInitial() async {
        guard !hasLoaded else { return }
        hasLoaded = true
        await refresh()
    }

    /// Re-fetches the first page and replaces the list (pull-to-refresh).
    public func refresh() async {
        isLoading = true
        defer { isLoading = false }
        do {
            let page = try await client.feed(limit: max(pageSize, items.count), offset: 0)
            total = page.total
            items = page.items
            errorMessage = nil
        } catch {
            errorMessage = Self.errorText(error)
        }
    }

    /// Appends the next page when scrolled to the bottom.
    public func loadMore() async {
        guard hasMore, !isLoading, !isLoadingMore else { return }
        isLoadingMore = true
        defer { isLoadingMore = false }
        do {
            let page = try await client.feed(limit: pageSize, offset: items.count)
            total = page.total
            var known = Set(items.map(\.id))
            for item in page.items where known.insert(item.id).inserted {
                items.append(item)
            }
        } catch {
            errorMessage = Self.errorText(error)
        }
    }

    /// Votes on an item (1 up / -1 down / 0 clear) with an optimistic update.
    public func vote(_ key: String, _ value: Int) async {
        guard let index = items.firstIndex(where: { $0.id == key }) else { return }
        let previousVote = items[index].vote
        items[index].vote = value
        do {
            let response = try await client.vote(key, value: value)
            if value == -1, response.vote == 0 {
                // The server removes downvoted items from the feed.
                items.removeAll { $0.id == key }
                total = max(0, total - 1)
            } else if let index = items.firstIndex(where: { $0.id == key }) {
                items[index].vote = response.vote
                items[index].saved = response.saved
            }
        } catch {
            if let index = items.firstIndex(where: { $0.id == key }) {
                items[index].vote = previousVote
            }
            errorMessage = Self.errorText(error)
        }
    }

    /// Toggles the saved state with an optimistic update.
    public func toggleSaved(_ key: String) async {
        guard let index = items.firstIndex(where: { $0.id == key }) else { return }
        let target = !items[index].saved
        let previous = items[index].saved
        items[index].saved = target
        do {
            let response = try await client.save(key, value: target)
            if let index = items.firstIndex(where: { $0.id == key }) {
                items[index].saved = response.saved
                items[index].vote = response.vote
            }
        } catch {
            if let index = items.firstIndex(where: { $0.id == key }) {
                items[index].saved = previous
            }
            errorMessage = Self.errorText(error)
        }
    }

    /// Reports the item as seen, once per item per session.
    public func markSeen(_ key: String) async {
        guard seenKeys.insert(key).inserted else { return }
        _ = try? await client.markSeen(key)
    }

    private static func errorText(_ error: Error) -> String {
        (error as? FeedError)?.errorDescription ?? error.localizedDescription
    }
}
