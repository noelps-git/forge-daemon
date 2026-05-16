// Package keychain wraps the macOS `security` CLI for generic-password storage.
// Using the CLI avoids CGO and keeps the binary portable to all Go toolchains.
package keychain

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

const defaultService = "forge-daemon"

// Set stores secret under service/account in the macOS Keychain.
// If the item already exists it is updated.
func Set(service, account, secret string) error {
	// Try to add first; if it already exists, update it.
	cmd := exec.Command(
		"security", "add-generic-password",
		"-s", service,
		"-a", account,
		"-w", secret,
		"-U", // -U = update if exists
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("keychain set: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// Get retrieves the password for service/account from the macOS Keychain.
// Returns an error when the item is not found.
func Get(service, account string) (string, error) {
	cmd := exec.Command(
		"security", "find-generic-password",
		"-s", service,
		"-a", account,
		"-w", // print only the password to stdout
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("keychain get: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimRight(stdout.String(), "\n"), nil
}

// Delete removes the keychain item for service/account.
// Returns nil if the item did not exist.
func Delete(service, account string) error {
	cmd := exec.Command(
		"security", "delete-generic-password",
		"-s", service,
		"-a", account,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// exit 44 = item not found — treat as success
		if strings.Contains(stderr.String(), "could not be found") {
			return nil
		}
		return fmt.Errorf("keychain delete: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// SetAnthropicKey is a convenience wrapper that stores the Anthropic API key.
func SetAnthropicKey(key string) error { return Set(defaultService, "anthropic", key) }

// GetAnthropicKey retrieves the stored Anthropic API key.
func GetAnthropicKey() (string, error) { return Get(defaultService, "anthropic") }

// SetOllamaKey stores the Ollama API token (if used with auth).
func SetOllamaKey(key string) error { return Set(defaultService, "ollama", key) }

// GetOllamaKey retrieves the stored Ollama API token.
func GetOllamaKey() (string, error) { return Get(defaultService, "ollama") }
