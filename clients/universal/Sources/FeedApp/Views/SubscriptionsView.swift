import SwiftUI
#if canImport(FeedCore)
import FeedCore
#endif

/// Manage subscriptions: add/remove feeds, per-feed notify policy, and a
/// manual "refresh all feeds" action.
@MainActor
struct SubscriptionsView: View {
    @State private var vm: SubscriptionsViewModel

    init(client: FeedClient) {
        _vm = State(initialValue: SubscriptionsViewModel(client: client))
    }

    var body: some View {
        List {
            Section {
                if vm.subscriptions.isEmpty {
                    if vm.isLoading {
                        HStack {
                            Spacer()
                            ProgressView("Loading subscriptions…")
                            Spacer()
                        }
                    } else {
                        Text("No subscriptions yet — add your first feed below.")
                            .foregroundStyle(.secondary)
                    }
                } else {
                    ForEach(vm.subscriptions) { subscription in
                        SubscriptionRow(
                            subscription: subscription,
                            onPolicyChange: { policy in
                                Task { await vm.setNotify(subscription, policy) }
                            },
                            onDelete: {
                                Task { await vm.delete(subscription) }
                            }
                        )
                    }
                }
            }

            Section("Add Feed") {
                HStack(spacing: 10) {
                    TextField("https://example.com/feed.xml", text: $vm.newFeedURL)
                        .textFieldStyle(.roundedBorder)
                        #if os(iOS)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                        .keyboardType(.URL)
                        #endif
                    Button {
                        Task { await vm.add() }
                    } label: {
                        if vm.isAdding {
                            ProgressView()
                                .controlSize(.small)
                        } else {
                            Text("Add")
                        }
                    }
                    .disabled(vm.isAdding || vm.newFeedURL.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
                }
                .padding(.vertical, 2)
            }

            Section {
                Button {
                    Task { await vm.refreshFeeds() }
                } label: {
                    if vm.isRefreshing {
                        HStack {
                            ProgressView()
                                .controlSize(.small)
                            Text("Fetching all feeds…")
                        }
                    } else {
                        Label("Refresh All Feeds Now", systemImage: "arrow.clockwise")
                    }
                }
                .disabled(vm.isRefreshing || vm.subscriptions.isEmpty)
            }

            if let notice = vm.notice {
                Section {
                    Label(notice, systemImage: "info.circle")
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                }
            }
        }
        .navigationTitle("Subscriptions")
        .task { await vm.load() }
        .refreshable { await vm.refresh() }
    }
}

@MainActor
private struct SubscriptionRow: View {
    let subscription: Subscription
    let onPolicyChange: (Subscription.NotifyPolicy) -> Void
    let onDelete: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack {
                Text(subscription.displayTitle)
                    .font(.body.weight(.medium))
                    .lineLimit(1)
                Spacer()
                Button(role: .destructive, action: onDelete) {
                    Image(systemName: "trash")
                }
                .buttonStyle(.borderless)
                .hint("Remove this feed and all of its items")
            }

            HStack(spacing: 6) {
                Text(subscription.url)
                    .lineLimit(1)
                if let itemCount = subscription.itemCount, itemCount > 0 {
                    Text("·")
                    Text("\(itemCount) item\(itemCount == 1 ? "" : "s")")
                }
            }
            .font(.caption)
            .foregroundStyle(.secondary)

            Picker("Notify", selection: Binding(
                get: { subscription.notifyPolicy },
                set: { onPolicyChange($0) }
            )) {
                ForEach(Subscription.NotifyPolicy.allCases, id: \.self) { policy in
                    Text(policy.label).tag(policy)
                }
            }
            .pickerStyle(.segmented)
            .labelsHidden()
            .frame(maxWidth: 220, alignment: .leading)
            .hint("When to send push notifications for this source")
        }
        .padding(.vertical, 2)
    }
}
