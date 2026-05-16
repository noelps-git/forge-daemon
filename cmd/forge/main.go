// Command forge is the CLI entry point for the Forge daemon.
//
// Subcommands:
//
//	forge start          - start the daemon (blocks)
//	forge stop           - send SIGTERM to running daemon
//	forge status         - query /health
//	forge version        - print version
//	forge keys set --anthropic KEY
//	forge keys get --anthropic
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"syscall"

	"github.com/forgeapp/forge-daemon/internal/config"
	"github.com/forgeapp/forge-daemon/internal/daemon"
	"github.com/forgeapp/forge-daemon/internal/keychain"
)

// Version is injected at link time via -ldflags "-X main.Version=x.y.z".
var Version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "start":
		cmdStart()
	case "stop":
		cmdStop()
	case "status":
		cmdStatus()
	case "version", "--version", "-v":
		fmt.Println("forge version", Version)
	case "keys":
		cmdKeys(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "forge: unknown command %q\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, `forge — AI bridge daemon for Xcode and Android Studio

Usage:
  forge start                        Start the daemon (blocks)
  forge stop                         Stop a running daemon
  forge status                       Print daemon health
  forge version                      Print version
  forge keys set --anthropic KEY     Store Anthropic API key in Keychain
  forge keys get --anthropic         Retrieve Anthropic API key from Keychain`)
}

func cmdStart() {
	cfg, err := config.Load()
	if err != nil {
		fatal("load config: %v", err)
	}
	srv := daemon.New(cfg)
	if err := srv.Start(); err != nil {
		fatal("%v", err)
	}
}

func cmdStop() {
	cfg, err := config.Load()
	if err != nil {
		fatal("load config: %v", err)
	}
	pidFile := cfg.Daemon.PidFile
	data, err := os.ReadFile(pidFile)
	if err != nil {
		fatal("cannot read pid file %s: %v\n(is the daemon running?)", pidFile, err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		fatal("invalid pid in %s: %v", pidFile, err)
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		fatal("cannot find process %d: %v", pid, err)
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		fatal("cannot signal process %d: %v", pid, err)
	}
	fmt.Printf("forge: sent SIGTERM to pid %d\n", pid)
}

func cmdStatus() {
	cfg, err := config.Load()
	if err != nil {
		fatal("load config: %v", err)
	}
	url := fmt.Sprintf("http://127.0.0.1:%d/health", cfg.Daemon.Port)
	resp, err := http.Get(url) //nolint:gosec
	if err != nil {
		fmt.Fprintf(os.Stderr, "forge: daemon not reachable at %s\n", url)
		os.Exit(1)
	}
	defer resp.Body.Close()

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		fatal("decode response: %v", err)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(result)
}

func cmdKeys(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: forge keys (set|get) --anthropic [KEY]")
		os.Exit(1)
	}
	subCmd := args[0]
	rest := args[1:]

	fs := flag.NewFlagSet("keys", flag.ExitOnError)
	anthropicKey := fs.String("anthropic", "", "Anthropic API key")
	ollamaKey := fs.String("ollama", "", "Ollama API token")
	_ = fs.Parse(rest)

	switch subCmd {
	case "set":
		if *anthropicKey != "" {
			if err := keychain.SetAnthropicKey(*anthropicKey); err != nil {
				fatal("keychain set anthropic: %v", err)
			}
			fmt.Println("forge: Anthropic API key stored in Keychain")
		}
		if *ollamaKey != "" {
			if err := keychain.SetOllamaKey(*ollamaKey); err != nil {
				fatal("keychain set ollama: %v", err)
			}
			fmt.Println("forge: Ollama token stored in Keychain")
		}
		if *anthropicKey == "" && *ollamaKey == "" {
			fmt.Fprintln(os.Stderr, "forge keys set: specify --anthropic KEY or --ollama KEY")
			os.Exit(1)
		}
	case "get":
		if *anthropicKey != "" || len(rest) == 0 {
			// Default to anthropic if flag present (empty value = was provided) or no flags.
			key, err := keychain.GetAnthropicKey()
			if err != nil {
				fatal("keychain get anthropic: %v", err)
			}
			fmt.Println(key)
		}
		if *ollamaKey != "" {
			key, err := keychain.GetOllamaKey()
			if err != nil {
				fatal("keychain get ollama: %v", err)
			}
			fmt.Println(key)
		}
	default:
		fmt.Fprintf(os.Stderr, "forge keys: unknown subcommand %q (valid: set, get)\n", subCmd)
		os.Exit(1)
	}
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "forge: "+format+"\n", args...)
	os.Exit(1)
}
