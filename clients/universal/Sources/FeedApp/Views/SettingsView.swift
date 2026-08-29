import SwiftUI
#if canImport(FeedCore)
import FeedCore
#endif

/// Server connection (URL + token, sign out) and Memos integration settings.
@MainActor
struct SettingsView: View {
    @Environment(AppState.self) private var appState

    @State private var vm: SettingsViewModel

    @State private var serverURLInput = ""
    @State private var tokenInput = ""
    @State private var signInError: String?
    @State private var isSigningIn = false

    init(client: FeedClient) {
        _vm = State(initialValue: SettingsViewModel(client: client))
    }

    var body: some View {
        Form {
            serverSection
            memosSection
            aboutSection
        }
        .formStyle(.grouped)
        .navigationTitle("Settings")
        .task {
            serverURLInput = appState.serverURL
            tokenInput = appState.token
            await vm.load()
        }
    }

    // MARK: - Server connection

    private var serverSection: some View {
        Section("Server") {
            TextField("Server URL", text: $serverURLInput)
                #if os(iOS)
                .textInputAutocapitalization(.never)
                .autocorrectionDisabled()
                .keyboardType(.URL)
                #endif

            SecureField("Access token", text: $tokenInput)

            if let signInError {
                Text(signInError)
                    .font(.footnote)
                    .foregroundStyle(.red)
            }

            HStack {
                Button {
                    Task { await signIn() }
                } label: {
                    if isSigningIn {
                        ProgressView()
                            .controlSize(.small)
                    } else {
                        Text("Sign In")
                    }
                }
                .disabled(isSigningIn || serverURLInput.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty || tokenInput.isEmpty)

                Spacer()

                Button("Sign Out", role: .destructive) {
                    appState.signOut()
                }
            }
        }
    }

    // MARK: - Memos

    private var memosSection: some View {
        Section {
            TextField("Memos URL", text: $vm.memosURL, prompt: Text("https://memos.example.com"))
                #if os(iOS)
                .textInputAutocapitalization(.never)
                .autocorrectionDisabled()
                .keyboardType(.URL)
                #endif

            SecureField("Memos token", text: $vm.memosToken)

            Button {
                Task { await vm.saveMemos() }
            } label: {
                if vm.isLoading {
                    ProgressView()
                        .controlSize(.small)
                } else {
                    Text("Save Memos Settings")
                }
            }
            .disabled(vm.isLoading)
        } header: {
            Text("Memos")
        } footer: {
            Text("Saved stories are mirrored to this Memos instance.")
        }
    }

    // MARK: - Misc

    private var aboutSection: some View {
        Section {
            if let notice = vm.notice {
                Label(notice, systemImage: "info.circle")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }
            LabeledContent("Server", value: appState.serverURL)
            if let settings = vm.settings, let lastSync = settings.memoLastSyncAt,
               let date = RFC3339.parse(lastSync) {
                LabeledContent("Last Memos Sync", value: date.formatted(date: .abbreviated, time: .shortened))
            }
        } header: {
            Text("Status")
        } footer: {
            Text("Feed · native client for the feed2 self-hosted reader.")
        }
    }

    private func signIn() async {
        isSigningIn = true
        defer { isSigningIn = false }
        do {
            try await appState.signIn(serverURL: serverURLInput, token: tokenInput)
            signInError = nil
        } catch {
            signInError = (error as? FeedError)?.errorDescription ?? error.localizedDescription
        }
    }
}
