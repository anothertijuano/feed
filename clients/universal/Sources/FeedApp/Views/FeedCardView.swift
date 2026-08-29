import SwiftUI
#if canImport(FeedCore)
import FeedCore
#endif

/// An Instagram-like vertical card for one feed item: optional hero image,
/// title, source + relative time, excerpt, and a vote/save action row.
@MainActor
struct FeedCardView: View {
    let item: FeedItem
    /// Tapping the card body opens the article.
    var onOpen: () -> Void = {}
    /// Called with 1 (up), -1 (down) or 0 (clear).
    var onVote: (Int) -> Void = { _ in }
    var onToggleSaved: () -> Void = {}

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            if let mediaURL = item.firstMediaURL {
                CachedAsyncImage(url: mediaURL)
                    .frame(maxWidth: .infinity)
                    .frame(height: mediaHeight)
                    .clipped()
            }

            VStack(alignment: .leading, spacing: 8) {
                Text(item.title)
                    .font(.headline)
                    .lineLimit(3)
                    .multilineTextAlignment(.leading)

                HStack(spacing: 6) {
                    Text(item.sourceName)
                    Text("·")
                    if let date = item.displayDate {
                        Text(date, format: .relative(presentation: .named))
                    }
                }
                .font(.caption)
                .foregroundStyle(.secondary)

                if !item.excerpt.isEmpty {
                    Text(item.excerpt)
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                        .lineLimit(4)
                        .multilineTextAlignment(.leading)
                }

                actionRow
            }
            .padding(.horizontal, 14)
            .padding(.vertical, 12)
        }
        .background(
            RoundedRectangle(cornerRadius: 16, style: .continuous)
                .fill(Color.cardBackground)
        )
        .clipShape(RoundedRectangle(cornerRadius: 16, style: .continuous))
        .contentShape(RoundedRectangle(cornerRadius: 16, style: .continuous))
        .onTapGesture { onOpen() }
        .contextMenu {
            Button("Open in Browser") { onOpen() }
            Button(item.saved ? "Remove from Saved" : "Save") { onToggleSaved() }
        }
    }

    private var mediaHeight: CGFloat {
        item.firstMediaURL == nil ? 0 : 220
    }

    private var actionRow: some View {
        HStack(spacing: 18) {
            voteButton(systemImage: "arrow.up", active: item.vote == 1) {
                onVote(item.vote == 1 ? 0 : 1)
            }
            voteButton(systemImage: "arrow.down", active: item.vote == -1) {
                onVote(item.vote == -1 ? 0 : -1)
            }
            Spacer()
            Button(action: onToggleSaved) {
                Image(systemName: item.saved ? "bookmark.fill" : "bookmark")
                    .font(.system(size: 15, weight: .semibold))
                    .foregroundStyle(item.saved ? Color.accentColor : Color.secondary)
            }
            .buttonStyle(.plain)
            .hint(item.saved ? "Remove from saved" : "Save")
        }
    }

    private func voteButton(systemImage: String, active: Bool, action: @escaping () -> Void) -> some View {
        Button(action: action) {
            Image(systemName: systemImage)
                .font(.system(size: 15, weight: .semibold))
                .foregroundStyle(active ? Color.accentColor : Color.secondary)
        }
        .buttonStyle(.plain)
        .hint(systemImage == "arrow.up" ? "Vote up" : "Vote down")
    }
}
