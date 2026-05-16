import Foundation
import XcodeKit

// MARK: - SourceEditorExtension

/// Principal class for the Forge Xcode Source Editor Extension.
/// Registered via NSExtensionPrincipalClass in Info.plist.
final class SourceEditorExtension: NSObject, XCSourceEditorExtension {

    /// Called once when the extension loads. Warm up the health check so the
    /// container app's status indicator can reflect daemon availability quickly.
    func extensionDidFinishLaunching() {
        ForgeClient.shared.checkHealth { isAlive in
            // Post a Darwin notification so ForgeApp can update its status view.
            let name = isAlive
                ? "dev.forgeapp.ForgeXcode.daemonOnline"
                : "dev.forgeapp.ForgeXcode.daemonOffline"
            DistributedNotificationCenter.default().postNotificationName(
                NSNotification.Name(name),
                object: nil,
                deliverImmediately: true
            )
        }
    }

    /// The list of commands surfaced under Editor > Forge in Xcode's menu.
    var commandDefinitions: [[XCSourceEditorCommandDefinitionKey: Any]] {
        [
            [
                .identifierKey: "dev.forgeapp.ForgeXcode.ForgeExtension.AskForgeCommand",
                .nameKey: "Ask Forge",
                .classNameKey: "ForgeExtension.AskForgeCommand"
            ],
            [
                .identifierKey: "dev.forgeapp.ForgeXcode.ForgeExtension.ExplainSelectionCommand",
                .nameKey: "Explain Selection",
                .classNameKey: "ForgeExtension.ExplainSelectionCommand"
            ],
            [
                .identifierKey: "dev.forgeapp.ForgeXcode.ForgeExtension.FixCurrentErrorCommand",
                .nameKey: "Fix Current Error",
                .classNameKey: "ForgeExtension.FixCurrentErrorCommand"
            ]
        ]
    }
}
