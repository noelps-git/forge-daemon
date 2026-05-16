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
)

type ollamaProvider struct {
	model    string
	endpoint string
	client   *http.Client
}

func newOllama(cfg *config.Config) (Provider, error) {
	endpoint := strings.TrimRight(cfg.Privacy.OllamaEndpoint, "/")
	if endpoint == "" {
		endpoint = "http://localhost:11434"
	}
	model := cfg.AI.ChatModel
	if model == "" {
		model = "llama3"
	}
	return &ollamaProvider{
		model:    model,
		endpoint: endpoint,
		client:   &http.Client{},
	}, nil
}

func (o *ollamaProvider) Name() string  { return "ollama" }
func (o *ollamaProvider) Model() string { return o.model }

type ollamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

type ollamaResponse struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
}

func (o *ollamaProvider) Ask(ctx context.Context, req AskRequest) (<-chan string, error) {
	prompt := buildOllamaPrompt(req)

	body := ollamaRequest{
		Model:  o.model,
		Prompt: prompt,
		Stream: true,
	}
	rawBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("ollama: marshal request: %w", err)
	}

	url := o.endpoint + "/api/generate"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(rawBody))
	if err != nil {
		return nil, fmt.Errorf("ollama: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama: HTTP request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama: error %d: %s", resp.StatusCode, string(errBody))
	}

	ch := make(chan string, 32)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}
			var ev ollamaResponse
			if err := json.Unmarshal([]byte(line), &ev); err != nil {
				continue
			}
			if ev.Response != "" {
				select {
				case ch <- ev.Response:
				case <-ctx.Done():
					return
				}
			}
			if ev.Done {
				return
			}
		}
	}()

	return ch, nil
}

func buildOllamaPrompt(req AskRequest) string {
	var sb strings.Builder
	sb.WriteString("You are Forge, an AI coding assistant embedded in the developer's IDE.\n")
	if len(req.Context) > 0 {
		sb.WriteString("## Project context\n\n")
		for _, f := range req.Context {
			sb.WriteString(fmt.Sprintf("### %s\n```\n%s\n```\n\n", f.Path, f.Content))
		}
	}
	sb.WriteString("## Question\n\n")
	sb.WriteString(req.Instruction)
	return sb.String()
}
