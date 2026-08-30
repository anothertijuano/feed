import Foundation

// MARK: - Media

/// One image inside a content item.
public struct Media: Codable, Hashable, Sendable {
    public let src: String
    /// Whether the image should be letterboxed rather than cropped.
    public let contain: Bool

    public init(src: String, contain: Bool = false) {
        self.src = src
        self.contain = contain
    }

    private enum CodingKeys: String, CodingKey {
        case src
        case contain
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        src = try container.decode(String.self, forKey: .src)
        contain = try container.decodeIfPresent(Bool.self, forKey: .contain) ?? false
    }
}

// MARK: - Feed item

/// A piece of content in the feed, annotated with the current user's
/// interaction state (`vote` / `saved`).
public struct FeedItem: Codable, Hashable, Identifiable, Sendable {
    public let id: String
    public let title: String
    public let link: String
    public let sourceName: String
    public let media: [Media]
    public let paragraphs: [String]
    public let subscription: String
    public let guid: String?
    /// When the item entered the feed.
    public let fetchedAt: String
    /// The article's own publication date, when the feed provides one.
    public let publishedAt: String?
    /// The current user's vote: -1, 0 or 1 (the server stores downvotes as 0).
    public var vote: Int
    public var saved: Bool

    public init(
        id: String,
        title: String,
        link: String,
        sourceName: String,
        media: [Media] = [],
        paragraphs: [String] = [],
        subscription: String = "",
        guid: String? = nil,
        fetchedAt: String = "",
        publishedAt: String? = nil,
        vote: Int = 0,
        saved: Bool = false
    ) {
        self.id = id
        self.title = title
        self.link = link
        self.sourceName = sourceName
        self.media = media
        self.paragraphs = paragraphs
        self.subscription = subscription
        self.guid = guid
        self.fetchedAt = fetchedAt
        self.publishedAt = publishedAt
        self.vote = vote
        self.saved = saved
    }

    private enum CodingKeys: String, CodingKey {
        case id, title, link, sourceName, media, paragraphs
        case subscription, guid, fetchedAt, publishedAt, vote, saved
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        id = try container.decode(String.self, forKey: .id)
        title = try container.decode(String.self, forKey: .title)
        link = try container.decode(String.self, forKey: .link)
        sourceName = try container.decode(String.self, forKey: .sourceName)
        media = try container.decodeIfPresent([Media].self, forKey: .media) ?? []
        paragraphs = try container.decodeIfPresent([String].self, forKey: .paragraphs) ?? []
        subscription = try container.decode(String.self, forKey: .subscription)
        guid = try container.decodeIfPresent(String.self, forKey: .guid)
        fetchedAt = try container.decode(String.self, forKey: .fetchedAt)
        publishedAt = try container.decodeIfPresent(String.self, forKey: .publishedAt)
        vote = try container.decodeIfPresent(Int.self, forKey: .vote) ?? 0
        saved = try container.decodeIfPresent(Bool.self, forKey: .saved) ?? false
    }

    public var fetchedAtDate: Date? { RFC3339.parse(fetchedAt) }
    public var publishedAtDate: Date? { publishedAt.flatMap(RFC3339.parse) }
    /// The best date to show for the item: publication date, else fetch date.
    public var displayDate: Date? { publishedAtDate ?? fetchedAtDate }
    public var hostName: String? { URL(string: link)?.host }

    /// First non-empty paragraph, used as a card excerpt.
    public var excerpt: String {
        paragraphs.first(where: { !$0.isEmpty }) ?? ""
    }

    public var firstMediaURL: URL? {
        guard let src = media.first?.src else { return nil }
        return URL(string: src)
    }
}

// MARK: - Pages / lists

/// Response of `GET /api/feed`.
public struct FeedPage: Codable, Sendable {
    public let total: Int
    public let items: [FeedItem]
}

/// Response of `GET /api/saved`.
public struct ItemList: Codable, Sendable {
    public let items: [FeedItem]
}

// MARK: - Interactions

/// Response of `POST /api/interactions`.
public struct InteractionResponse: Codable, Sendable {
    public let key: String
    public let vote: Int
    public let saved: Bool
}

// MARK: - Subscriptions

/// An RSS/Atom/JSON feed the server polls.
public struct Subscription: Codable, Hashable, Identifiable, Sendable {
    public let id: String
    public let url: String
    public let title: String?
    public let etag: String?
    public let lastModified: String?
    public let addedAt: String?
    public let lastFetchedAt: String?
    public let itemCount: Int?
    public let lastError: String?
    /// Push-notification policy: "default" (rank-based), "always" or "never".
    public let notify: String?

    public enum NotifyPolicy: String, CaseIterable, Codable, Sendable {
        case `default`
        case always
        case never

        public var label: String {
            switch self {
            case .default: "Default"
            case .always: "Always"
            case .never: "Never"
            }
        }
    }

    public init(
        id: String,
        url: String,
        title: String? = nil,
        etag: String? = nil,
        lastModified: String? = nil,
        addedAt: String? = nil,
        lastFetchedAt: String? = nil,
        itemCount: Int? = nil,
        lastError: String? = nil,
        notify: String? = nil
    ) {
        self.id = id
        self.url = url
        self.title = title
        self.etag = etag
        self.lastModified = lastModified
        self.addedAt = addedAt
        self.lastFetchedAt = lastFetchedAt
        self.itemCount = itemCount
        self.lastError = lastError
        self.notify = notify
    }

    private enum CodingKeys: String, CodingKey {
        case id, url, title, etag, lastModified, addedAt
        case lastFetchedAt, itemCount, lastError, notify
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        id = try container.decode(String.self, forKey: .id)
        url = try container.decode(String.self, forKey: .url)
        title = try container.decodeIfPresent(String.self, forKey: .title)
        etag = try container.decodeIfPresent(String.self, forKey: .etag)
        lastModified = try container.decodeIfPresent(String.self, forKey: .lastModified)
        addedAt = try container.decodeIfPresent(String.self, forKey: .addedAt)
        lastFetchedAt = try container.decodeIfPresent(String.self, forKey: .lastFetchedAt)
        itemCount = try container.decodeIfPresent(Int.self, forKey: .itemCount)
        lastError = try container.decodeIfPresent(String.self, forKey: .lastError)
        notify = try container.decodeIfPresent(String.self, forKey: .notify)
    }

    public var notifyPolicy: NotifyPolicy {
        NotifyPolicy(rawValue: notify ?? "") ?? .default
    }

    public var displayTitle: String {
        if let title, !title.isEmpty { return title }
        return URL(string: url)?.host ?? url
    }

    /// A copy of this subscription with a different notify policy.
    public func withNotify(_ policy: NotifyPolicy) -> Subscription {
        Subscription(
            id: id, url: url, title: title, etag: etag, lastModified: lastModified,
            addedAt: addedAt, lastFetchedAt: lastFetchedAt, itemCount: itemCount,
            lastError: lastError, notify: policy.rawValue
        )
    }
}

/// Response of `GET /api/subscriptions`.
public struct SubscriptionList: Codable, Sendable {
    public let items: [Subscription]
}

// MARK: - Settings

/// Server settings (Memos integration).
public struct ServerSettings: Codable, Hashable, Sendable {
    public let memosUrl: String
    public let memosToken: String
    public let memoLastSyncAt: String?
    public let memoLastError: String?

    public init(
        memosUrl: String = "",
        memosToken: String = "",
        memoLastSyncAt: String? = nil,
        memoLastError: String? = nil
    ) {
        self.memosUrl = memosUrl
        self.memosToken = memosToken
        self.memoLastSyncAt = memoLastSyncAt
        self.memoLastError = memoLastError
    }

    private enum CodingKeys: String, CodingKey {
        case memosUrl, memosToken, memoLastSyncAt, memoLastError
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        memosUrl = try container.decodeIfPresent(String.self, forKey: .memosUrl) ?? ""
        memosToken = try container.decodeIfPresent(String.self, forKey: .memosToken) ?? ""
        memoLastSyncAt = try container.decodeIfPresent(String.self, forKey: .memoLastSyncAt)
        memoLastError = try container.decodeIfPresent(String.self, forKey: .memoLastError)
    }
}

// MARK: - Access tokens

/// An access token entry as listed by `GET /api/tokens` (never contains the
/// raw token itself).
public struct APIToken: Codable, Hashable, Identifiable, Sendable {
    public let id: String
    public let name: String
    public let createdAt: String
}

/// Response of `POST /api/tokens`: includes the raw token exactly once.
public struct CreatedToken: Codable, Hashable, Sendable {
    public let token: String
    public let id: String
    public let name: String
    public let createdAt: String
}

/// Response of `GET /api/tokens`.
public struct TokenList: Codable, Sendable {
    public let items: [APIToken]
}

// MARK: - Misc responses

/// Response of `POST /api/refresh`.
public struct RefreshResponse: Codable, Sendable {
    public let new: Int
}

/// Response of `DELETE /api/subscriptions/{id}`.
public struct RemovedResponse: Codable, Sendable {
    public let removed: Int?
}

/// Generic `{"status": "ok"}` response.
public struct StatusResponse: Codable, Sendable {
    public let status: String?
}
