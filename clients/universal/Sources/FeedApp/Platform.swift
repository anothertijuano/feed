import SwiftUI
#if os(macOS)
import AppKit
#else
import UIKit
#endif

/// Opens a URL in the default browser.
@MainActor
public func openExternalURL(_ url: URL) {
    #if os(macOS)
    NSWorkspace.shared.open(url)
    #else
    UIApplication.shared.open(url, options: [:], completionHandler: nil)
    #endif
}

extension View {
    /// Platform-safe tooltip/hint (`.help` is not available everywhere).
    @ViewBuilder
    func hint(_ text: LocalizedStringKey) -> some View {
        #if os(macOS)
        self.help(text)
        #else
        self
        #endif
    }
}

@MainActor
extension Image {
    /// Builds an `Image` from raw image data (via NSImage/UIImage).
    init?(data: Data) {
        #if os(macOS)
        guard let image = NSImage(data: data) else { return nil }
        self.init(nsImage: image)
        #else
        guard let image = UIImage(data: data) else { return nil }
        self.init(uiImage: image)
        #endif
    }
}

extension Color {
    /// Muted, dark-mode-friendly card background.
    static var cardBackground: Color {
        #if os(macOS)
        Color(nsColor: .controlBackgroundColor)
        #else
        Color(uiColor: .secondarySystemGroupedBackground)
        #endif
    }

    /// Screen background behind the feed lists.
    static var screenBackground: Color {
        #if os(macOS)
        Color(nsColor: .windowBackgroundColor)
        #else
        Color(uiColor: .systemGroupedBackground)
        #endif
    }
}
