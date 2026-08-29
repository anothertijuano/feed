import Foundation
import Observation
#if canImport(FeedCore)
import FeedCore
#endif

/// State and actions for subscription management.
@MainActor
@Observable
public final class SubscriptionsViewModel {
    public private(set) var subscriptions: [Subscription] = []
    public private(set) var isLoading = false
    public private(set) var isAdding = false
    public private(set) var isRefreshing = false
    /// Transient success/error notice (e.g. "Added aljazeera.com").
    public private(set) var notice: String?
    /// Input field for the "add feed" form.
    public var newFeedURL = ""

    private let client: FeedClient

    public init(client: FeedClient) {
        self.client = client
    }

    public func load() async {
        guard subscriptions.isEmpty else { return }
        await refresh()
    }

    public func refresh() async {
        isLoading = true
        defer { isLoading = false }
        do {
            subscriptions = try await client.subscriptions()
            notice = nil
        } catch {
            notice = Self.errorText(error)
        }
    }

    /// Adds the feed currently in `newFeedURL`.
    public func add() async {
        let url = newFeedURL.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !url.isEmpty else { return }
        isAdding = true
        defer { isAdding = false }
        do {
            let sub = try await client.addSubscription(url: url)
            newFeedURL = ""
            await refresh()
            notice = "Added “\(sub.displayTitle)”"
        } catch {
            notice = Self.errorText(error)
        }
    }

    /// Removes a subscription (and, server-side, all of its items).
    public func delete(_ subscription: Subscription) async {
        subscriptions.removeAll { $0.id == subscription.id }
        do {
            _ = try await client.deleteSubscription(id: subscription.id)
            notice = nil
        } catch {
            await refresh() // reconcile with the server
            notice = Self.errorText(error)
        }
    }

    /// Updates the push-notification policy of a subscription.
    public func setNotify(_ subscription: Subscription, _ policy: Subscription.NotifyPolicy) async {
        guard let index = subscriptions.firstIndex(where: { $0.id == subscription.id }) else { return }
        subscriptions[index] = subscription.withNotify(policy)
        do {
            let updated = try await client.setNotify(id: subscription.id, policy: policy)
            if let index = subscriptions.firstIndex(where: { $0.id == subscription.id }) {
                subscriptions[index] = updated
            }
            notice = nil
        } catch {
            if let index = subscriptions.firstIndex(where: { $0.id == subscription.id }) {
                subscriptions[index] = subscription
            }
            notice = Self.errorText(error)
        }
    }

    /// Asks the server to fetch all feeds now.
    public func refreshFeeds() async {
        isRefreshing = true
        defer { isRefreshing = false }
        do {
            let count = try await client.refresh()
            notice = count == 1 ? "Fetched 1 new item" : "Fetched \(count) new items"
        } catch {
            notice = Self.errorText(error)
        }
    }

    private static func errorText(_ error: Error) -> String {
        (error as? FeedError)?.errorDescription ?? error.localizedDescription
    }
}
