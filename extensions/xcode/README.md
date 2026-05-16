# Forge for Xcode — Source Editor Extension

Ask Claude questions about your code without leaving Xcode.  
Connects to the local Forge daemon at `http://localhost:7878`.

---

## Quick start

### 1. Make sure the daemon is running

```bash
forge start
```

Verify it is up:

```bash
curl http://localhost:7878/health
# {"status":"ok","version":"0.1.0"}
```

### 2. Open the project in Xcode

```
open extensions/xcode/ForgeXcode.xcodeproj
```

### 3. Set your development team

1. Select the **ForgeXcode** project in the navigator.
2. Choose the **ForgeXcode** target → Signing & Capabilities.
3. Set **Team** to your Apple Developer account.
4. Repeat for the **ForgeExtension** target.

### 4. Build and run (⌘R)

This launches the **Forge for Xcode** container app.  
The app shows daemon connection status and a button to open System Settings.

### 5. Enable the extension

**System Settings → Privacy & Security → Extensions → Xcode Source Editor**

Tick **Forge Extension**.

### 6. Use the commands in Xcode

Open any Swift (or other source) file, then in the menu bar:

| Command | Location | What it does |
|---|---|---|
| **Ask Forge** | Editor ▸ Forge ▸ Ask Forge | Type any question; response inserted as comments |
| **Explain Selection** | Editor ▸ Forge ▸ Explain Selection | Select code; explanation shown in a scrollable alert |
| **Fix Current Error** | Editor ▸ Forge ▸ Fix Current Error | Fetches build errors and applies a fix in-editor |

### 7. Set a keyboard shortcut (optional)

Xcode → Settings → Key Bindings → search "Forge" → assign your preferred shortcut.  
A popular choice is **⌃⌥⌘F** for Ask Forge.

---

## Architecture

```
ForgeXcode.xcodeproj
├── ForgeApp/          Container macOS app (status UI)
│   ├── AppDelegate.swift
│   ├── Info.plist
│   └── ForgeXcode.entitlements
└── ForgeExtension/    Xcode Source Editor Extension
    ├── SourceEditorExtension.swift   Principal class, command declarations
    ├── ForgeClient.swift             HTTP client for daemon API
    ├── Commands.swift                AskForge, ExplainSelection, FixCurrentError
    ├── Info.plist
    └── ForgeExtension.entitlements
```

## Daemon API (reference)

| Method | Path | Auth | Purpose |
|--------|------|------|---------|
| POST | `/ide/v1/connect` | — | Obtain session token |
| POST | `/ide/v1/ask` | `X-Forge-Token` | Ask Claude |
| GET | `/ide/v1/errors` | `X-Forge-Token` | Fetch build errors |
| GET | `/health` | — | Liveness check |

## Troubleshooting

**"Not connected to Forge daemon"**  
Run `forge start` in your terminal and click Refresh in the Forge app.

**Extension not appearing in Editor menu**  
Make sure the extension is enabled in System Settings → Extensions → Xcode Source Editor.  
Quit and relaunch Xcode after enabling.

**Sandbox / network error**  
Both targets require `com.apple.security.network.client = true` in their entitlements (already set).  
If you see `POSIX error 1` or `Operation not permitted`, rebuild after a `Clean Build Folder` (⇧⌘K).

**Response inserted in wrong place**  
The extension inserts responses after the last selection range.  
Make sure you have a text selection (or at least a cursor position) before invoking a command.
