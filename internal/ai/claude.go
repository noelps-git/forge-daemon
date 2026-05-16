package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/forgeapp/forge-daemon/internal/config"
	"github.com/forgeapp/forge-daemon/internal/keychain"
)

const (
	anthropicAPIURL     = "https://api.anthropic.com/v1/messages"
	anthropicAPIVersion = "2023-06-01"
	claudeMaxTokens     = 8192
)

type claudeProvider struct {
	model  string
	apiKey string
	client *http.Client
}

func newClaude(cfg *config.Config) (Provider, error) {
	key, err := keychain.GetAnthropicKey()
	if err != nil {
		return nil, fmt.Errorf("claude: cannot retrieve API key from keychain: %w", err)
	}
	if key == "" {
		return nil, fmt.Errorf("claude: Anthropic API key not set (run: forge keys set --anthropic KEY)")
	}
	return &claudeProvider{
		model:  cfg.AI.ChatModel,
		apiKey: key,
		client: &http.Client{},
	}, nil
}

func (c *claudeProvider) Name() string  { return "claude" }
func (c *claudeProvider) Model() string { return c.model }

// claudeRequest is the JSON body sent to the Anthropic Messages API.
type claudeRequest struct {
	Model     string           `json:"model"`
	MaxTokens int              `json:"max_tokens"`
	Stream    bool             `json:"stream"`
	System    string           `json:"system"`
	Messages  []claudeMessage  `json:"messages"`
}

type claudeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// SSE event shapes we care about.
type claudeSSEEvent struct {
	Type  string          `json:"type"`
	Delta *claudeSSEDelta `json:"delta,omitempty"`
}

type claudeSSEDelta struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func (c *claudeProvider) Ask(ctx context.Context, req AskRequest) (<-chan string, error) {
	system := buildSystemPrompt(req.Context)

	body := claudeRequest{
		Model:     c.model,
		MaxTokens: claudeMaxTokens,
		Stream:    true,
		System:    system,
		Messages: []claudeMessage{
			{Role: "user", Content: req.Instruction},
		},
	}

	rawBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("claude: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, anthropicAPIURL, bytes.NewReader(rawBody))
	if err != nil {
		return nil, fmt.Errorf("claude: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", anthropicAPIVersion)

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("claude: HTTP request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("claude: API error %d: %s", resp.StatusCode, string(errBody))
	}

	ch := make(chan string, 32)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			payload := strings.TrimPrefix(line, "data: ")
			if payload == "[DONE]" {
				break
			}
			var ev claudeSSEEvent
			if err := json.Unmarshal([]byte(payload), &ev); err != nil {
				continue
			}
			if ev.Type == "content_block_delta" && ev.Delta != nil && ev.Delta.Type == "text_delta" {
				select {
				case ch <- ev.Delta.Text:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return ch, nil
}

// buildSystemPrompt constructs the system message, embedding any file context.
func buildSystemPrompt(files []ContextFile) string {
	var sb strings.Builder
	sb.WriteString("You are Forge, an AI coding assistant embedded in the developer's IDE.\n")
	sb.WriteString("You have access to the project files listed below.\n")
	sb.WriteString("Provide concise, accurate answers. When suggesting code changes, emit unified diffs.\n\n")

	if len(files) == 0 {
		return sb.String()
	}

	sb.WriteString("## Project context\n\n")
	for _, f := range files {
		sb.WriteString(fmt.Sprintf("### %s\n```\n%s\n```\n\n", f.Path, f.Content))
	}
	return sb.String()
}
