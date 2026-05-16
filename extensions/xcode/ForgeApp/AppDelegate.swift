import AppKit

// MARK: - AppDelegate

@main
final class AppDelegate: NSObject, NSApplicationDelegate {

    private var window: NSWindow!
    private var statusLabel: NSTextField!
    private var statusDot: NSView!

    // MARK: - Application Lifecycle

    func applicationDidFinishLaunching(_ notification: Notification) {
        buildWindow()
        checkDaemonStatus()

        // Listen for status notifications posted by the extension
        DistributedNotificationCenter.default().addObserver(
            self,
            selector: #selector(daemonCameOnline),
            name: NSNotification.Name("dev.forgeapp.ForgeXcode.daemonOnline"),
            object: nil
        )
        DistributedNotificationCenter.default().addObserver(
            self,
            selector: #selector(daemonWentOffline),
            name: NSNotification.Name("dev.forgeapp.ForgeXcode.daemonOffline"),
            object: nil
        )
    }

    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool {
        return true
    }

    // MARK: - Window Construction

    private func buildWindow() {
        let width: CGFloat  = 480
        let height: CGFloat = 340

        window = NSWindow(
            contentRect: NSRect(x: 0, y: 0, width: width, height: height),
            styleMask: [.titled, .closable, .miniaturizable],
            backing: .buffered,
            defer: false
        )
        window.title = "Forge for Xcode"
        window.center()
        window.isReleasedWhenClosed = false

        guard let contentView = window.contentView else { return }
        contentView.wantsLayer = true
        contentView.layer?.backgroundColor = NSColor.windowBackgroundColor.cgColor

        // ── Logo / Title ──────────────────────────────────────────────────
        let titleLabel = makeLabel(
            "Forge for Xcode",
            fontSize: 22,
            weight: .bold
        )
        titleLabel.frame = NSRect(x: 24, y: height - 60, width: width - 48, height: 32)
        contentView.addSubview(titleLabel)

        let subtitleLabel = makeLabel(
            "AI pair programmer — powered by Claude",
            fontSize: 13,
            weight: .regular,
            color: .secondaryLabelColor
        )
        subtitleLabel.frame = NSRect(x: 24, y: height - 88, width: width - 48, height: 20)
        contentView.addSubview(subtitleLabel)

        // ── Separator ─────────────────────────────────────────────────────
        let sep = NSBox()
        sep.boxType = .separator
        sep.frame = NSRect(x: 24, y: height - 100, width: width - 48, height: 1)
        contentView.addSubview(sep)

        // ── Status Row ────────────────────────────────────────────────────
        let statusTitle = makeLabel("Daemon status:", fontSize: 13, weight: .semibold)
        statusTitle.frame = NSRect(x: 24, y: height - 140, width: 140, height: 20)
        contentView.addSubview(statusTitle)

        // Coloured dot
        statusDot = NSView(frame: NSRect(x: 170, y: height - 137, width: 12, height: 12))
        statusDot.wantsLayer = true
        statusDot.layer?.cornerRadius = 6
        statusDot.layer?.backgroundColor = NSColor.systemGray.cgColor
        contentView.addSubview(statusDot)

        statusLabel = makeLabel("Checking…", fontSize: 13, weight: .regular)
        statusLabel.frame = NSRect(x: 190, y: height - 140, width: 180, height: 20)
        contentView.addSubview(statusLabel)

        let refreshButton = NSButton(
            title: "Refresh",
            target: self,
            action: #selector(refreshTapped)
        )
        refreshButton.bezelStyle = .rounded
        refreshButton.frame = NSRect(x: width - 108, y: height - 145, width: 84, height: 28)
        contentView.addSubview(refreshButton)

        // ── Instructions ──────────────────────────────────────────────────
        let instructionsTitle = makeLabel(
            "How to use",
            fontSize: 13,
            weight: .semibold
        )
        instructionsTitle.frame = NSRect(x: 24, y: height - 180, width: width - 48, height: 20)
        contentView.addSubview(instructionsTitle)

        let steps = [
            "1.  Enable the extension in System Settings (button below).",
            "2.  Open any project in Xcode.",
            "3.  Use  Editor ▸ Forge ▸ Ask Forge  (or bind a shortcut in",
            "      Xcode ▸ Key Bindings).",
            "4.  Make sure the daemon is running:  forge start"
        ]
        var y = height - 210
        for step in steps {
            let lbl = makeLabel(step, fontSize: 12, weight: .regular, color: .secondaryLabelColor)
            lbl.frame = NSRect(x: 24, y: y, width: width - 48, height: 18)
            contentView.addSubview(lbl)
            y -= 18
        }

        // ── Open System Settings button ───────────────────────────────────
        let settingsButton = NSButton(
            title: "Open Extensions in System Settings",
            target: self,
            action: #selector(openExtensionSettings)
        )
        settingsButton.bezelStyle = .rounded
        settingsButton.frame = NSRect(
            x: (width - 280) / 2,
            y: 20,
            width: 280,
            height: 32
        )
        contentView.addSubview(settingsButton)

        window.makeKeyAndOrderFront(nil)
    }

    // MARK: - Label Factory

    private func makeLabel(
        _ text: String,
        fontSize: CGFloat,
        weight: NSFont.Weight,
        color: NSColor = .labelColor
    ) -> NSTextField {
        let label = NSTextField(labelWithString: text)
        label.font = NSFont.systemFont(ofSize: fontSize, weight: weight)
        label.textColor = color
        label.lineBreakMode = .byWordWrapping
        return label
    }

    // MARK: - Daemon Health

    private func checkDaemonStatus() {
        ForgeClientBridge.checkHealth { [weak self] isAlive in
            DispatchQueue.main.async {
                self?.updateStatusUI(online: isAlive)
            }
        }
    }

    private func updateStatusUI(online: Bool) {
        if online {
            statusLabel.stringValue = "Connected (localhost:7878)"
            statusDot.layer?.backgroundColor = NSColor.systemGreen.cgColor
        } else {
            statusLabel.stringValue = "Not running — run forge start"
            statusDot.layer?.backgroundColor = NSColor.systemRed.cgColor
        }
    }

    // MARK: - Actions

    @objc private func refreshTapped() {
        statusLabel.stringValue = "Checking…"
        statusDot.layer?.backgroundColor = NSColor.systemGray.cgColor
        checkDaemonStatus()
    }

    @objc private func openExtensionSettings() {
        // macOS 13+: open the Extensions pane directly
        if let url = URL(string: "x-apple.systempreferences:com.apple.ExtensionsPreferences") {
            NSWorkspace.shared.open(url)
        }
    }

    @objc private func daemonCameOnline() {
        DispatchQueue.main.async { self.updateStatusUI(online: true) }
    }

    @objc private func daemonWentOffline() {
        DispatchQueue.main.async { self.updateStatusUI(online: false) }
    }
}

// MARK: - ForgeClientBridge
// A lightweight duplicate of the health-check logic so the app target doesn't
// need to link the extension's ForgeClient (different bundle, same logic).

enum ForgeClientBridge {
    static func checkHealth(completion: @escaping (Bool) -> Void) {
        guard let url = URL(string: "http://localhost:7878/health") else {
            completion(false)
            return
        }

        var request = URLRequest(url: url)
        request.httpMethod = "GET"
        request.timeoutInterval = 5

        let config = URLSessionConfiguration.ephemeral
        let session = URLSession(configuration: config)

        struct HealthResponse: Decodable { let status: String }

        session.dataTask(with: request) { data, response, _ in
            guard
                let http = response as? HTTPURLResponse,
                (200...299).contains(http.statusCode),
                let data,
                let parsed = try? JSONDecoder().decode(HealthResponse.self, from: data),
                parsed.status == "ok"
            else {
                completion(false)
                return
            }
            completion(true)
        }.resume()
    }
}
