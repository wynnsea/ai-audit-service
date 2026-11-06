package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

)

type Config struct {
	ChatBaseURL   string
	APIKey        string
	ChatModel     string
	Timeout       time.Duration
	EmbedModel    string
	EmbedBaseURL  string
	RerankBaseURL string
	RerankModel	  string			
}

type Client struct {
	cfg  Config
	http *http.Client
}

func NewClient(cfg Config) *Client {
	cfg.ChatBaseURL = strings.TrimRight(cfg.ChatBaseURL, "/")
	cfg.EmbedBaseURL = strings.TrimRight(cfg.EmbedBaseURL, "/")
	cfg.RerankBaseURL = strings.TrimRight(cfg.RerankBaseURL, "/")
	return &Client{
		cfg: cfg,
		// LLM lokal bisa lambat, apalagi model reasoning
		http: &http.Client{Timeout: cfg.Timeout},
	}
}

func (c *Client) post(ctx context.Context, url, model string, reqBody, respBody any) error {
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	// gateway Bifrost membuang field non-standar (misal chat_template_kwargs untuk mematikan
	// thinking Qwen) kecuali header ini dikirim; vLLM langsung mengabaikannya
	req.Header.Set("x-bf-passthrough-extra-params", "true")
	if c.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	}

	start := time.Now()
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("ai %s: %w", url, err)
	}
	defer resp.Body.Close()
	log.Printf("ai %s model=%q %d %s", url, model, resp.StatusCode, time.Since(start).Round(time.Millisecond))

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("ai %s: status %d: %s", url, resp.StatusCode, body)
	}

	if err := json.NewDecoder(resp.Body).Decode(respBody); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}
