package ai

import (
	"context"
	"strings"
	"time"

	"github.com/forgeapp/forge-daemon/internal/config"
)

const nullResponse = "This is a mock response from the null AI provider. " +
	"Replace this with a real provider (claude or ollama) in your config."

type nullProvider struct {
	model string
}

func newNull(_ *config.Config) Provider {
	return &nullProvider{model: "null-v1"}
}

func (n *nullProvider) Name() string  { return "null" }
func (n *nullProvider) Model() string { return n.model }

// Ask streams the canned response token by token with a 10 ms delay between tokens.
func (n *nullProvider) Ask(ctx context.Context, req AskRequest) (<-chan string, error) {
	tokens := strings.Fields(nullResponse)
	ch := make(chan string, len(tokens))

	go func() {
		defer close(ch)
		for _, tok := range tokens {
			select {
			case <-ctx.Done():
				return
			case <-time.After(10 * time.Millisecond):
				select {
				case ch <- tok + " ":
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return ch, nil
}
