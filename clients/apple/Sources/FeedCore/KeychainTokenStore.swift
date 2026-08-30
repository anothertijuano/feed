import Foundation
import Security

/// Stores the API token in the Keychain (macOS and iOS share the Security
/// framework). When the Keychain is unavailable — e.g. unsigned command-line
/// builds on macOS — reads and writes transparently fall back to UserDefaults
/// so the app keeps working.
public struct KeychainTokenStore: Sendable {
    private let service = "com.anothertijuano.feed2"
    private let account = "api-token"
    private let defaultsKey = "com.anothertijuano.feed2.api-token"

    public init() {}

    /// Returns the stored token, or nil when absent.
    public func read() -> String? {
        var query = baseQuery
        query[kSecReturnData as String] = true
        query[kSecMatchLimit as String] = kSecMatchLimitOne
        var item: CFTypeRef?
        let status = SecItemCopyMatching(query as CFDictionary, &item)
        if status == errSecSuccess, let data = item as? Data,
           let token = String(data: data, encoding: .utf8) {
            return token
        }
        // Keychain failed or the item was never written there: try the
        // UserDefaults fallback (e.g. a previous run without keychain access).
        return UserDefaults.standard.string(forKey: defaultsKey)
    }

    /// Stores (or replaces) the token.
    public func write(_ token: String) {
        let data = Data(token.utf8)
        let query = baseQuery
        let lookupStatus = SecItemCopyMatching(query as CFDictionary, nil)
        switch lookupStatus {
        case errSecSuccess:
            let attributes = [kSecValueData as String: data] as CFDictionary
            if SecItemUpdate(query as CFDictionary, attributes) == errSecSuccess {
                UserDefaults.standard.removeObject(forKey: defaultsKey)
                return
            }
        case errSecItemNotFound:
            var attributes = query
            attributes[kSecValueData as String] = data
            attributes[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlock
            if SecItemAdd(attributes as CFDictionary, nil) == errSecSuccess {
                UserDefaults.standard.removeObject(forKey: defaultsKey)
                return
            }
        default:
            break
        }
        // Keychain write failed: fall back to UserDefaults.
        UserDefaults.standard.set(token, forKey: defaultsKey)
    }

    /// Removes the token everywhere it may live.
    public func delete() {
        SecItemDelete(baseQuery as CFDictionary)
        UserDefaults.standard.removeObject(forKey: defaultsKey)
    }

    private var baseQuery: [String: Any] {
        [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account,
        ]
    }
}
