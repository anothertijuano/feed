import Foundation
import Observation
#if canImport(FeedCore)
import FeedCore
#endif

/// Root session state: the persisted server URL (UserDefaults) and access
/// token (Keychain), and the `FeedClient` derived from them.
@MainActor
@Observable
public final class AppState {
    public private(set) var client: FeedClient?
    public private(set) var isSignedIn = false

    private let tokenStore = KeychainTokenStore()

    private enum Keys {
        static let serverURL = "com.anothertijuano.feed2.server-url"
    }

    /// The currently configured server URL (empty when not set).
    public var serverURL: String {
        UserDefaults.standard.string(forKey: Keys.serverURL) ?? ""
    }

    /// The currently stored access token (empty when not set).
    public var token: String {
        tokenStore.read() ?? ""
    }

    public init() {
        let rawURL = UserDefaults.standard.string(forKey: Keys.serverURL) ?? ""
        let token = tokenStore.read() ?? ""
        if !rawURL.isEmpty, !token.isEmpty, let url = Self.normalizedURL(rawURL) {
            client = FeedClient(baseURL: url, token: token)
            isSignedIn = true
        }
    }

    /// Validates the credentials against the server, then persists them and
    /// switches to the signed-in state. Throws `FeedError` on failure.
    public func signIn(serverURL rawURL: String, token rawToken: String) async throws {
        let trimmedURL = rawURL.trimmingCharacters(in: .whitespacesAndNewlines)
        let trimmedToken = rawToken.trimmingCharacters(in: .whitespacesAndNewlines)

        guard let url = Self.normalizedURL(trimmedURL) else {
            throw FeedError.badURL(trimmedURL)
        }
        guard !trimmedToken.isEmpty else {
            throw FeedError.notConfigured
        }

        let candidate = FeedClient(baseURL: url, token: trimmedToken)
        // A tiny authenticated request proves connectivity + credentials.
        _ = try await candidate.feed(limit: 1)

        UserDefaults.standard.set(url.absoluteString, forKey: Keys.serverURL)
        tokenStore.write(trimmedToken)
        client = candidate
        isSignedIn = true
    }

    public func signOut() {
        tokenStore.delete()
        UserDefaults.standard.removeObject(forKey: Keys.serverURL)
        client = nil
        isSignedIn = false
    }

    /// Accepts "https://host", "http://host" or a bare "host:port" and
    /// returns a normalized URL, or nil when invalid.
    public nonisolated static func normalizedURL(_ raw: String) -> URL? {
        var candidate = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !candidate.isEmpty else { return nil }
        if !candidate.contains("://") {
            candidate = "https://" + candidate
        }
        guard let url = URL(string: candidate),
              let scheme = url.scheme?.lowercased(),
              scheme == "http" || scheme == "https",
              url.host != nil else {
            return nil
        }
        return url
    }
}
