import Foundation

/// Parsing helpers for the RFC 3339 timestamps produced by the feed2 server
/// (e.g. "2026-08-29T18:10:23Z"), with or without fractional seconds.
public enum RFC3339 {
    public static func parse(_ string: String) -> Date? {
        if let date = try? Date(
            string,
            strategy: Date.ISO8601FormatStyle(includingFractionalSeconds: true)
        ) {
            return date
        }
        return try? Date(
            string,
            strategy: Date.ISO8601FormatStyle(includingFractionalSeconds: false)
        )
    }
}
