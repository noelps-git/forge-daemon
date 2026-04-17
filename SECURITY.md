# Security Policy

## Supported Versions

| Version | Supported |
|---------|-----------|
| Latest (main) | ✅ |

## Reporting a Vulnerability

**Do not open a public GitHub issue for security vulnerabilities.**

Email: **noelrajakumarps@gmail.com**

Include:
- Description of the vulnerability
- Steps to reproduce
- Potential impact
- Any suggested fix (optional)

You'll receive an acknowledgement within 48 hours and a resolution timeline within 7 days.

## Scope

| In scope | Out of scope |
|----------|--------------|
| Daemon HTTP server (`localhost:7878`) | Third-party dependencies |
| MCP WebSocket server | Social engineering |
| Keychain integration | Physical access attacks |
| Session token handling | Issues in forks |
| Path traversal in diff operations | |

## Security Design

- All IDE ↔ daemon communication is `localhost` only — never exposed externally
- API keys stored in macOS Keychain only — never written to disk, config files, or logs
- Session tokens are randomly generated UUIDs, rotated on every IDE connect
- All file operations validate paths are within the declared project root (no path traversal)
- No telemetry, no analytics, no data sent anywhere without explicit user action

## Responsible Disclosure

We follow a 90-day responsible disclosure policy. If a fix is not issued within 90 days, you may publish your findings.
