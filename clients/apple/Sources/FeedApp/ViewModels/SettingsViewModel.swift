import Foundation
import Observation
#if canImport(FeedCore)
import FeedCore
#endif

/// State and actions for server settings (Memos integration).
@MainActor
@Observable
public final class SettingsViewModel {
    public private(set) var settings: ServerSettings?
    public private(set) var isLoading = false
    public private(set) var notice: String?
    /// Editable copies of the server-side Memos configuration.
    public var memosURL = ""
    public var memosToken = ""

    private let client: FeedClient

    public init(client: FeedClient) {
        self.client = client
    }

    public func load() async {
        isLoading = true
        defer { isLoading = false }
        do {
            let settings = try await client.settings()
            self.settings = settings
            memosURL = settings.memosUrl
            memosToken = settings.memosToken
        } catch {
            notice = Self.errorText(error)
        }
    }

    public func saveMemos() async {
        isLoading = true
        defer { isLoading = false }
        do {
            let updated = try await client.updateSettings(
                memosUrl: memosURL.trimmingCharacters(in: .whitespacesAndNewlines),
                memosToken: memosToken.trimmingCharacters(in: .whitespacesAndNewlines)
            )
            settings = updated
            memosURL = updated.memosUrl
            memosToken = updated.memosToken
            notice = "Memos settings saved."
        } catch {
            notice = Self.errorText(error)
        }
    }

    private static func errorText(_ error: Error) -> String {
        (error as? FeedError)?.errorDescription ?? error.localizedDescription
    }
}
