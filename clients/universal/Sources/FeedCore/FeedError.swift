import Foundation

/// Errors thrown by `FeedClient`.
public enum FeedError: Error, LocalizedError, Sendable {
    /// No server URL / token configured.
    case notConfigured
    /// A URL was malformed or unsupported.
    case badURL(String)
    /// The server answered with a 4xx/5xx status. `message` carries the
    /// server's `{"error": "…"}` text when available.
    case server(status: Int, message: String)
    /// A transport-level failure (no connection, timeout, TLS, …).
    case network(String)
    /// The response was not a valid HTTP response.
    case invalidResponse
    /// The response body could not be decoded.
    case decoding(String)

    public var errorDescription: String? {
        switch self {
        case .notConfigured:
            "No server configured. Enter a server URL and access token."
        case .badURL(let url):
            "“\(url)” is not a valid http(s) server URL."
        case .server(let status, let message):
            message.isEmpty ? "Server error (HTTP \(status))." : message
        case .network(let message):
            message
        case .invalidResponse:
            "The server returned an unexpected response."
        case .decoding(let message):
            "Could not read the server response (\(message))."
        }
    }
}
