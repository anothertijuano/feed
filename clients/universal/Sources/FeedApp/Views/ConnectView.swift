import SwiftUI
#if canImport(FeedCore)
import FeedCore
#endif

/// First-run / signed-out screen: enter the self-hosted server URL and an
/// access token, then validate and connect.
@MainActor
struct ConnectView: View {
    @Environment(AppState.self) private var appState

    @State private var serverURL = ""
    @State private var token = ""
    @State private var isBusy = false
    @State private var errorMessage: String?

    var body: some View {
        VStack(spacing: 20) {
            VStack(spacing: 8) {
                Image(systemName: "dot.radiowaves.left.and.right")
                    .font(.system(size: 44))
                    .foregroundStyle(.tint)
                Text("Feed")
                    .font(.largeTitle.bold())
                Text("Connect to your self-hosted feed2 server")
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
            }
            .padding(.top, 16)

            VStack(spacing: 12) {
                TextField(
                    "Server URL",
                    text: $serverURL,
                    prompt: Text("https://feed.example.com")
                )
                .textContentType(.URL)

                SecureField(
                    "Access token",
                    text: $token,
                    prompt: Text("ft_…")
                )

                if let errorMessage {
                    Text(errorMessage)
                        .font(.callout)
                        .foregroundStyle(.red)
                        .frame(maxWidth: .infinity, alignment: .leading)
                }

                Button {
                    Task { await connect() }
                } label: {
                    if isBusy {
                        ProgressView()
                            .frame(maxWidth: .infinity)
                    } else {
                        Text("Connect")
                            .frame(maxWidth: .infinity)
                    }
                }
                .buttonStyle(.borderedProminent)
                .controlSize(.large)
                .disabled(isBusy || trimmedURL.isEmpty || token.isEmpty)
                .keyboardShortcut(.defaultAction)
            }
            .textFieldStyle(.roundedBorder)

            Text("Create an access token in the web UI: Settings → Access tokens.")
                .font(.footnote)
                .foregroundStyle(.secondary)
                .multilineTextAlignment(.center)
        }
        .padding(32)
        .frame(maxWidth: 440)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        #if os(iOS)
        .textInputAutocapitalization(.never)
        .autocorrectionDisabled()
        .keyboardType(.URL)
        #endif
    }

    private var trimmedURL: String {
        serverURL.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    private func connect() async {
        isBusy = true
        defer { isBusy = false }
        do {
            try await appState.signIn(serverURL: serverURL, token: token)
            errorMessage = nil
        } catch {
            errorMessage = (error as? FeedError)?.errorDescription ?? error.localizedDescription
        }
    }
}
