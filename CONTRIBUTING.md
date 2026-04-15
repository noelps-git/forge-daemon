# Contributing to Forge

Thanks for wanting to help. Forge is early — contributions have outsized impact right now.

## Before you start

Open an issue first if you're planning something significant. Saves everyone time.

## Setup

```bash
git clone https://github.com/forgeapp/forge-daemon
cd forge-daemon
xcode-select --install   # for go-sqlite3
go mod download
make test                 # all tests should pass
```

## Making changes

- One thing per PR. Small PRs get reviewed fast.
- Tests are not optional. New behaviour needs a test.
- Run `make ci` before pushing — it runs fmt, lint, and tests with the race detector.

```bash
make ci
```

## What we need most right now

**High value, well-scoped:**
- Better Swift symbol extraction (beyond regex — lightweight AST)
- Kotlin/Java symbol extraction
- Xcode `.xcresult` build log parser
- Gradle structured output parser

**Good first issues:**
- Add `forge keys list` command to show what keys are stored
- Add `forge logs` command to tail daemon logs
- Improve error messages when daemon port is already in use

## Code style

- `gofmt` — enforced by CI
- Error strings lowercase, no punctuation: `"file not found"` not `"File not found."`
- No `panic` in library code — return errors
- Log with `zerolog`, not `fmt.Println`

## PR checklist

- [ ] `make ci` passes
- [ ] New behaviour has a test
- [ ] No secrets, API keys, or real file paths in the diff
- [ ] Updated relevant docs if behaviour changed

## Questions

Open an issue or email `dev@forgeapp.dev`.
