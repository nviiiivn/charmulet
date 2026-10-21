// Package ollama streams completions from an ollama server.
package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/nviiiivn/charmulet/internal/core/provider"
)

var _ provider.Provider = (*Client)(nil)

type Client struct {
	Base string
	hc   *http.Client

	mu     sync.Mutex
	models []string
}

func New(base string) *Client {
	return &Client{
		Base: strings.TrimRight(base, "/"),
		hc:   &http.Client{Timeout: 0},
	}
}

func (c *Client) Name() string { return "ollama" }

type tagResp struct {
	Models []struct {
		Name string `json:"name"`
	} `json:"models"`
}

func (c *Client) Models() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.models != nil {
		return c.models
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Base+"/api/tags", nil)
	if err != nil {
		return nil
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var tr tagResp
	if json.NewDecoder(resp.Body).Decode(&tr) != nil {
		return nil
	}
	out := make([]string, 0, len(tr.Models))
	for _, m := range tr.Models {
		out = append(out, m.Name)
	}
	c.models = out
	return out
}

// chatChunk is one NDJSON line of the /api/chat stream.
type chatChunk struct {
	Message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"message"`
	Done bool `json:"done"`
}

func (c *Client) Complete(ctx context.Context, model string, msgs []provider.Message) (provider.Stream, error) {
	payload, err := json.Marshal(map[string]any{
		"model":    model,
		"messages": msgs,
		"stream":   true,
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Base+"/api/chat", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama %s: %w", c.Base, err)
	}
	if resp.StatusCode != http.StatusOK {
		body := new(bytes.Buffer)
		body.ReadFrom(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("ollama %s: HTTP %d: %s", c.Base, resp.StatusCode, strings.TrimSpace(body.String()))
	}

	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	return func() (string, bool, error) {
		for sc.Scan() {
			var cc chatChunk
			if err := json.Unmarshal(sc.Bytes(), &cc); err != nil {
				resp.Body.Close()
				return "", false, err
			}
			if cc.Done {
				resp.Body.Close()
				return "", false, nil
			}
			if cc.Message.Content != "" {
				return cc.Message.Content, true, nil
			}
		}
		err := sc.Err()
		resp.Body.Close()
		if err == nil {
			return "", false, nil
		}
		if ctx.Err() != nil {
			return "", false, nil // cancelled — treat as clean end
		}
		return "", false, err
	}, nil
}
