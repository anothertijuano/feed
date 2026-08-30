import Foundation

/// Async/await client for the feed2 HTTP API.
///
/// Every call attaches `Authorization: Bearer <token>`. Errors from the
/// server (JSON `{"error": "…"}` with a 4xx/5xx status) surface as
/// `FeedError.server(status:message:)`.
public actor FeedClient {
    public let baseURL: URL

    private let token: String
    private let session: URLSession
    private let decoder = JSONDecoder()
    private let encoder = JSONEncoder()

    /// - Parameters:
    ///   - baseURL: the server root, e.g. `https://feed.example.com`.
    ///   - token: the `ft_…` access token.
    public init(baseURL: URL, token: String, session: URLSession = .shared) {
        self.baseURL = baseURL
        self.token = token
        self.session = session
    }

    // MARK: - Feed

    /// `GET /api/feed?limit=N&offset=M`
    public func feed(limit: Int = 30, offset: Int = 0) async throws -> FeedPage {
        try await get("api/feed", query: [
            URLQueryItem(name: "limit", value: String(limit)),
            URLQueryItem(name: "offset", value: String(offset)),
        ])
    }

    /// `GET /api/saved` — newest save first.
    public func saved() async throws -> [FeedItem] {
        try await get("api/saved", as: ItemList.self).items
    }

    // MARK: - Interactions

    /// Votes on an item: 1 (up), -1 (down) or 0 (clear).
    @discardableResult
    public func vote(_ key: String, value: Int) async throws -> InteractionResponse {
        try await post("api/interactions", body: InteractionRequest(key: key, kind: "vote", value: .vote(value)))
    }

    /// Saves or unsaves an item.
    @discardableResult
    public func save(_ key: String, value: Bool) async throws -> InteractionResponse {
        try await post("api/interactions", body: InteractionRequest(key: key, kind: "save", value: .save(value)))
    }

    /// Marks an item as seen (stops counting presentations).
    @discardableResult
    public func markSeen(_ key: String) async throws -> InteractionResponse {
        try await post("api/interactions", body: InteractionRequest(key: key, kind: "seen", value: .seen(true)))
    }

    // MARK: - Subscriptions

    /// `GET /api/subscriptions`
    public func subscriptions() async throws -> [Subscription] {
        try await get("api/subscriptions", as: SubscriptionList.self).items
    }

    /// `POST /api/subscriptions` — adds a feed and returns the new subscription.
    public func addSubscription(url: String) async throws -> Subscription {
        try await post("api/subscriptions", body: SubscriptionURLBody(url: url))
    }

    /// `POST /api/subscriptions/{id}` — sets the push-notification policy.
    public func setNotify(id: String, policy: Subscription.NotifyPolicy) async throws -> Subscription {
        try await post("api/subscriptions/\(id)", body: NotifyBody(notify: policy.rawValue))
    }

    /// `DELETE /api/subscriptions/{id}`
    public func deleteSubscription(id: String) async throws {
        _ = try await send(RemovedResponse.self, method: "DELETE", path: "api/subscriptions/\(id)")
    }

    /// `POST /api/refresh` — fetches all subscriptions now; returns how many
    /// new items arrived.
    public func refresh() async throws -> Int {
        let response: RefreshResponse = try await post("api/refresh", body: EmptyBody())
        return response.new
    }

    // MARK: - Settings

    /// `GET /api/settings`
    public func settings() async throws -> ServerSettings {
        try await get("api/settings")
    }

    /// `POST /api/settings`
    public func updateSettings(memosUrl: String, memosToken: String) async throws -> ServerSettings {
        try await post("api/settings", body: MemosBody(memosUrl: memosUrl, memosToken: memosToken))
    }

    // MARK: - Access tokens

    /// `GET /api/tokens`
    public func tokens() async throws -> [APIToken] {
        try await get("api/tokens", as: TokenList.self).items
    }

    /// `POST /api/tokens` — creates a token; the raw token is returned exactly once.
    public func createToken(name: String) async throws -> CreatedToken {
        try await post("api/tokens", body: TokenNameBody(name: name))
    }

    /// `DELETE /api/tokens/{id}`
    public func deleteToken(id: String) async throws {
        _ = try await send(StatusResponse.self, method: "DELETE", path: "api/tokens/\(id)")
    }

    // MARK: - Plumbing

    private func get<T: Decodable>(
        _ path: String,
        query: [URLQueryItem] = [],
        as type: T.Type = T.self
    ) async throws -> T {
        try await send(T.self, method: "GET", path: path, query: query)
    }

    private func post<T: Decodable, B: Encodable>(_ path: String, body: B) async throws -> T {
        let data: Data
        do {
            data = try encoder.encode(body)
        } catch {
            throw FeedError.decoding(error.localizedDescription)
        }
        return try await send(T.self, method: "POST", path: path, body: data)
    }

    private func send<T: Decodable>(
        _ type: T.Type,
        method: String,
        path: String,
        query: [URLQueryItem] = [],
        body: Data? = nil
    ) async throws -> T {
        var components = URLComponents(url: baseURL.appendingPathComponent(path), resolvingAgainstBaseURL: false)
        if !query.isEmpty {
            components?.queryItems = query
        }
        guard let url = components?.url else {
            throw FeedError.badURL(path)
        }

        var request = URLRequest(url: url)
        request.httpMethod = method
        request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        request.setValue("application/json", forHTTPHeaderField: "Accept")
        if body != nil {
            request.setValue("application/json", forHTTPHeaderField: "Content-Type")
            request.httpBody = body
        }

        let data: Data
        let response: URLResponse
        do {
            (data, response) = try await session.data(for: request)
        } catch {
            throw FeedError.network(error.localizedDescription)
        }

        guard let http = response as? HTTPURLResponse else {
            throw FeedError.invalidResponse
        }
        guard (200..<300).contains(http.statusCode) else {
            throw FeedError.server(status: http.statusCode, message: Self.serverErrorMessage(data))
        }

        do {
            return try decoder.decode(T.self, from: data)
        } catch {
            throw FeedError.decoding(error.localizedDescription)
        }
    }

    private static func serverErrorMessage(_ data: Data) -> String {
        struct ErrorBody: Decodable {
            let error: String
        }
        return (try? JSONDecoder().decode(ErrorBody.self, from: data))?.error ?? ""
    }
}

// MARK: - Request bodies

private struct InteractionRequest: Encodable {
    let key: String
    let kind: String
    let value: InteractionValue
}

private enum InteractionValue: Encodable {
    case vote(Int)
    case save(Bool)
    case seen(Bool)

    func encode(to encoder: Encoder) throws {
        var container = encoder.singleValueContainer()
        switch self {
        case .vote(let value): try container.encode(value)
        case .save(let value): try container.encode(value)
        case .seen(let value): try container.encode(value)
        }
    }
}

private struct SubscriptionURLBody: Encodable {
    let url: String
}

private struct NotifyBody: Encodable {
    let notify: String
}

private struct MemosBody: Encodable {
    let memosUrl: String
    let memosToken: String
}

private struct TokenNameBody: Encodable {
    let name: String
}

private struct EmptyBody: Encodable {}
