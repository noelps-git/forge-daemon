# Forge — Android Studio Plugin

AI pair programmer for Android Studio. One keystroke connects you to your local
Forge daemon and brings Claude directly into your editor.

---

## 1. Prerequisites

The Forge daemon must be installed and running before the plugin can do anything.

```bash
# Install (if not already done)
cargo install forge-daemon          # or use the binary from the releases page

# Start the daemon (default port 7878)
forge start
```

Verify it is healthy:

```bash
curl http://localhost:7878/health
# → {"status":"ok","version":"0.1.0"}
```

---

## 2. Install from the JetBrains Marketplace

1. Open Android Studio.
2. Go to **File → Settings → Plugins** (or **Android Studio → Settings → Plugins** on macOS).
3. Switch to the **Marketplace** tab and search for **Forge**.
4. Click **Install**, then restart the IDE.

---

## 3. Build from Source

```bash
# Clone the repo
git clone https://github.com/forgeapp/forge-daemon.git
cd forge-daemon/extensions/android-studio

# Build the plugin zip
./gradlew buildPlugin

# The zip is written to:
#   build/distributions/forge-android-studio-0.1.0.zip
```

Install the zip:

1. Open Android Studio.
2. **File → Settings → Plugins → ⚙ → Install Plugin from Disk…**
3. Select the zip file you just built.
4. Restart the IDE.

### Gradle wrapper (first run only)

If you do not yet have the Gradle wrapper binaries:

```bash
gradle wrapper --gradle-version 8.6
```

---

## 4. Usage

### Right-click in the editor

1. Select some code (optional).
2. Right-click → **Forge** → choose an action:
   - **Ask Forge…** — type any question about your code.
   - **Explain Selection** — asks Forge to explain the highlighted code (requires a selection).
   - **Fix Current Error** — fetches the current build errors and asks Forge to fix one.

### Keyboard shortcut

| Platform | Shortcut |
|----------|----------|
| Windows / Linux | <kbd>Ctrl</kbd>+<kbd>Alt</kbd>+<kbd>F</kbd> |
| macOS | <kbd>⌘</kbd>+<kbd>⌥</kbd>+<kbd>F</kbd> |

This opens the **Ask Forge…** dialog directly.

### Tools menu

**Tools → Ask Forge…** launches the same dialog from the menu bar.

---

## 5. Tool Window

Open the Forge chat panel at any time:

**View → Tool Windows → Forge**

The panel lets you:

- Type questions in the text field at the bottom and press **Enter** or **Send**.
- See the full conversation history in the scrollable area above.
- Connect / reconnect to the daemon with the **Connect** button at the top.
- Monitor connection status ("Connected" / "Not connected").

---

## 6. Settings

**File → Settings → Tools → Forge AI**

| Setting | Default | Description |
|---------|---------|-------------|
| Daemon port | `7878` | Port the plugin uses to reach the daemon |
| Auto-connect on open | `true` | Automatically connect when a project is opened |

Click **Test Connection** to verify the daemon is reachable without leaving the settings panel.

---

## 7. Troubleshooting

| Symptom | Fix |
|---------|-----|
| "Forge daemon not running" notification | Run `forge start` in a terminal |
| HTTP 401 / token errors | Restart the daemon; the plugin will reconnect automatically |
| Plugin not loading | Ensure you are on Android Studio Ladybug (2024.2) or newer |
| Build fails: `androidStudio(…)` not found | Run `./gradlew dependencies` to download the IDE artifact |
