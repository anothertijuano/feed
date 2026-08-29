import Foundation
import Observation
#if canImport(FeedCore)
import FeedCore
#endif

/// State and actions for the saved-items list.
@MainActor
@Observable
public final class SavedViewModel {
    public private(set) var items: [FeedItem] = []
    public private(set) var isLoading = false
    public private(set) var errorMessage: String?

    private let client: FeedClient

    public init(client: FeedClient) {
        self.client = client
    }

    public func load() async {
        guard items.isEmpty else { return }
        await refresh()
    }

    public func refresh() async {
        isLoading = true
        defer { isLoading = false }
        do {
            items = try await client.saved()
            errorMessage = nil
        } catch {
            errorMessage = Self.errorText(error)
        }
    }

    /// Un-saves an item, removing it from the list optimistically.
    public func unsave(_ key: String) async {
        guard let index = items.firstIndex(where: { $0.id == key }) else { return }
        let removed = items.remove(at: index)
        do {
            _ = try await client.save(key, value: false)
        } catch {
            // Put the item back (at the front: the server list is
            // newest-save-first, but exact position is unknown).
            if !items.contains(where: { $0.id == key }) {
                items.insert(removed, at: 0)
            }
            errorMessage = Self.errorText(error)
        }
    }

    /// Votes on a saved item (a downvote unsaves + removes it server-side).
    public func vote(_ key: String, _ value: Int) async {
        guard let index = items.firstIndex(where: { $0.id == key }) else { return }
        let previousVote = items[index].vote
        items[index].vote = value
        do {
            let response = try await client.vote(key, value: value)
            if value == -1, response.vote == 0 {
                items.removeAll { $0.id == key }
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

    private static func errorText(_ error: Error) -> String {
        (error as? FeedError)?.errorDescription ?? error.localizedDescription
    }
}
