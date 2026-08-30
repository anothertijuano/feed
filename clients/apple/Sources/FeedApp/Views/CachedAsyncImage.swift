import Foundation
import SwiftUI

/// Cached, async image loader using a URLSession with a persistent
/// URLCache (memory + disk) — no external dependencies.
enum ImageLoader {
    static let session: URLSession = {
        let config = URLSessionConfiguration.default
        config.requestCachePolicy = .returnCacheDataElseLoad
        config.urlCache = URLCache(
            memoryCapacity: 64 << 20,
            diskCapacity: 256 << 20,
            diskPath: "feed-images"
        )
        return URLSession(configuration: config)
    }()

    static func load(_ url: URL) async throws -> Data {
        let (data, response) = try await session.data(from: url)
        guard let http = response as? HTTPURLResponse,
              (200..<300).contains(http.statusCode) else {
            throw URLError(.badServerResponse)
        }
        return data
    }
}

/// An image view that loads its content asynchronously from a URL and
/// caches it in the shared `ImageLoader` cache.
@MainActor
struct CachedAsyncImage: View {
    let url: URL

    @State private var phase: Phase = .loading

    private enum Phase {
        case loading
        case success(Image)
        case failed
    }

    var body: some View {
        Group {
            switch phase {
            case .success(let image):
                image
                    .resizable()
                    .aspectRatio(contentMode: .fill)
            case .loading:
                ZStack {
                    placeholder
                    ProgressView()
                }
            case .failed:
                placeholder
            }
        }
        .task(id: url) { await load() }
    }

    private var placeholder: some View {
        ZStack {
            Rectangle()
                .fill(Color.primary.opacity(0.06))
            Image(systemName: "photo")
                .font(.title2)
                .foregroundStyle(.tertiary)
        }
    }

    private func load() async {
        phase = .loading
        do {
            let data = try await ImageLoader.load(url)
            phase = Image(data: data).map(Phase.success) ?? .failed
        } catch {
            phase = .failed
        }
    }
}
