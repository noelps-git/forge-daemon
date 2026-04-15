<div align="center">

<br/>

```
███████╗ ██████╗ ██████╗  ██████╗ ███████╗
██╔════╝██╔═══██╗██╔══██╗██╔════╝ ██╔════╝
█████╗  ██║   ██║██████╔╝██║  ███╗█████╗  
██╔══╝  ██║   ██║██╔══██╗██║   ██║██╔══╝  
██║     ╚██████╔╝██║  ██║╚██████╔╝███████╗
╚═╝      ╚═════╝ ╚═╝  ╚═╝ ╚═════╝ ╚══════╝
```

**The AI bridge that Xcode and Android Studio never built.**

[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat-square&logo=go&logoColor=white)](https://go.dev)
[![License](https://img.shields.io/badge/License-MIT-brightgreen?style=flat-square)](LICENSE)
[![Status](https://img.shields.io/badge/Status-Sprint%200%20%E2%9C%93-orange?style=flat-square)](#roadmap)
[![MCP](https://img.shields.io/badge/Protocol-MCP%201.0-7C3AED?style=flat-square)](https://modelcontextprotocol.io)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen?style=flat-square)](CONTRIBUTING.md)

<br/>

*Every VS Code developer has Cursor. Every Xcode developer has… nothing.*  
*Forge fixes that.*

<br/>

</div>

---

<br/>

## The problem

You're building an iOS app. A build error lands. You:

1. Read the error in Xcode
2. Open a browser tab
3. Paste the error into Claude
4. Read the response
5. Copy the fix
6. Paste it back into Xcode
7. Repeat 40 times today

**Forge collapses that loop into one keystroke.**

<br/>

## What Forge is

A local daemon that runs on your Mac, speaks [Model Context Protocol](https://modelcontextprotocol.io), and bridges Claude directly into Xcode and Android Studio — the two major IDEs the AI tooling world forgot.

```
Your IDE  ──▶  Forge Daemon  ──▶  Claude API
   ▲                │                  │
   └────────────────┘◀─────────────────┘
        diffs, explanations, fixes
```

No cloud. No source code leaving your machine unless you choose it. No context-switching.

<br/>

## Features

| | Now | Sprint 2 | Sprint 4 |
|---|---|---|---|
| Xcode extension | 🔄 building | ✅ | ✅ |
| Android Studio plugin | 🔄 building | ✅ | ✅ |
| Fix build errors inline | 🔄 | ✅ | ✅ |
| Explain selected code | 🔄 | ✅ | ✅ |
| Multi-file refactor | — | — | ✅ |
| Privacy mode (local AI) | — | ✅ | ✅ |
| macOS companion app | 🔄 | ✅ | ✅ |
| MCP server (Claude Code) | ✅ | ✅ | ✅ |

<br/>

## Architecture

```
┌─────────────────────────────────────────────────┐
│                  Your Machine                    │
│                                                  │
│  ┌──────────┐    ┌─────────────────────────┐    │
│  │  Xcode   │    │      Forge Daemon        │    │
│  │Extension │◀──▶│  Go · localhost:7878     │    │
│  └──────────┘    │                         │    │
│                  │  ┌─────┐  ┌──────────┐  │    │
│  ┌──────────┐    │  │ MCP │  │  Index   │  │    │
│  │ Android  │◀──▶│  │ WS  │  │ SQLite   │  │    │
│  │  Studio  │    │  └─────┘  └──────────┘  │    │
│  └──────────┘    │                         │    │
│                  │  ┌─────────────────────┐│    │
│  ┌──────────┐    │  │   AI Providers       ││    │
│  │Companion │◀──▶│  │  Claude · Ollama     ││    │
│  │  App     │    │  └─────────────────────┘│    │
│  └──────────┘    └─────────────────────────┘    │
└─────────────────────────────────────────────────┘
```

The daemon is the only moving part. IDEs plug into it. AI models plug into it. You build the bridge once — every IDE and every model becomes a configuration choice.

<br/>

## Quickstart

```bash
# Install
brew tap forgeapp/forge
brew install forge

# Store your API key (Keychain — never written to disk)
forge keys set --anthropic YOUR_ANTHROPIC_KEY

# Start the daemon
forge start

# Verify
curl http://localhost:7878/health
```

```json
{
  "status": "ok",
  "version": "0.1.0",
  "ai_ready": true,
  "port": 7878
}
```

Then install the [Xcode extension](#) or [Android Studio plugin](#) and you're done.

<br/>

## For developers building on Forge

Forge exposes a full [MCP server](https://modelcontextprotocol.io) on `ws://localhost:7878/mcp`.  
Claude Code, custom agents, and any MCP-compatible client can connect directly.

```bash
# Connect Claude Code to your local Forge instance
claude mcp add forge ws://localhost:7878/mcp
```

Available MCP tools:

```
read_file      →  read any project file
apply_diff     →  apply unified diffs atomically
get_errors     →  current build errors
get_git_diff   →  current git state
list_files     →  project file tree
```

<br/>

## REST API

All IDE extensions talk to the daemon via a local REST API. No auth needed for health — session tokens for everything else.

```bash
# Connect your IDE
curl -X POST http://localhost:7878/ide/v1/connect \
  -H "Content-Type: application/json" \
  -d '{"ide":"xcode","project_path":"/path/to/MyApp"}'

# Ask Claude with context
curl -X POST http://localhost:7878/ide/v1/ask \
  -H "X-Forge-Token: frg_your_token" \
  -H "Content-Type: application/json" \
  -d '{"context_id":"ctx_abc","instruction":"Fix the crash on line 42","stream":true}'
```

Full API spec → [`docs/API-Spec.md`](docs/API-Spec.md)

<br/>

## Project structure

```
forge-daemon/
├── cmd/forge/              # CLI entry point (start · stop · status · version)
├── internal/
│   ├── ai/                 # AI providers (Claude · Ollama · null)
│   │   ├── claude.go       # Streaming + context builder + diff extractor
│   │   └── provider.go     # Provider interface + factory
│   ├── config/             # TOML config + env overrides
│   ├── daemon/             # HTTP server wiring + lifecycle
│   ├── ide/                # REST API handlers (20+ endpoints)
│   ├── index/              # SQLite project index + FSEvents watcher
│   ├── keychain/           # macOS Keychain integration
│   └── mcp/                # MCP WebSocket server
├── tests/                  # Integration + edge case tests (37 tests, Sprint 0)
├── Makefile
└── go.mod
```

<br/>

## Building from source

```bash
git clone https://github.com/forgeapp/forge-daemon
cd forge-daemon

# Dependencies (needs Xcode Command Line Tools for go-sqlite3)
xcode-select --install
go mod download

# Run tests
make test

# Build binary
make build

# Run in dev mode
make dev
```

<br/>

## Configuration

Config lives at `~/.forge/config.toml`. API keys are **never** written here — they live in macOS Keychain.

```toml
[daemon]
port = 7878
log_level = "info"

[ai]
default_provider = "claude"
chat_model = "claude-sonnet-4-5-20251001"
inline_model = "claude-haiku-4-5-20251001"
context_depth = "medium"        # minimal | medium | full
max_context_tokens = 80000

[privacy]
local_mode = false              # true = route to Ollama, nothing leaves machine
send_file_content = true
ollama_endpoint = "http://localhost:11434"
```

**Privacy mode** — set `local_mode = true` and point to a local [Ollama](https://ollama.ai) instance. Your source code never leaves your machine.

<br/>

## Roadmap

```
Sprint 0  ✅  Daemon · REST API · MCP server · Keychain auth · 37 tests
Sprint 1  🔄  Project indexer · FSEvents watcher · Symbol search
Sprint 2  ·   Context engine · Git diff · Build error parsing · Streaming
Sprint 3  ·   Diff apply · Atomic writes · Rollback
Sprint 4  ·   macOS companion app · Chat UI · Diff viewer
Sprint 5  ·   Android Studio / IntelliJ plugin
Sprint 6  ·   Xcode extension (App Store)
Sprint 7  ·   Privacy mode · Ollama · Auto-update
Sprint 8  ·   Paid plans · Cloud backend · Launch
```

Full sprint plan → [`docs/Roadmap.md`](docs/Roadmap.md)

<br/>

## Contributing

Forge is open source at its core. The daemon and IDE extensions are MIT licensed.

```bash
# Run tests before submitting a PR
make test-race

# Check formatting
make fmt

# Full CI check
make ci
```

Areas where contributions are most valuable right now:

- **Symbol extraction** — better Swift/Kotlin AST parsing beyond regex
- **Build log parsers** — Xcode `.xcresult` format, Gradle structured output  
- **Ollama models** — testing which local models work best for code tasks
- **Windows/Linux** — daemon works on all platforms, Keychain fallback needed

<br/>

## Security

- All IDE ↔ daemon communication is `localhost` only — no external exposure
- API keys stored in macOS Keychain, never in config files or logs
- Diff operations validate all paths are within project root — no path traversal
- Session tokens rotate on every IDE connect

Found a security issue? Email `security@forgeapp.dev` — please don't open a public issue.

<br/>

## License

Forge Daemon — [MIT License](LICENSE)  
Forge Companion App — Proprietary  
Forge Cloud Backend — Proprietary

<br/>

---

<div align="center">

Built by [Noel Rajakumar](https://github.com/noelrajakumar) and [Muthu Kumar](https://github.com/muthukumar) · Chennai, India

*"The best tools get out of your way."*

<br/>

⭐ Star this repo if Forge saves you a context switch today

</div>
