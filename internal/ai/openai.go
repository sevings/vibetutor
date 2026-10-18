package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OpenAI implements Client against an OpenAI-compatible
// POST {baseURL}/chat/completions endpoint.
type OpenAI struct {
	baseURL string
	apiKey  string
	model   string
	http    *http.Client
}

// NewOpenAI builds a client. The base URL must point at the API root,
// e.g. "https://api.openai.com/v1" or "http://localhost:8080/v1".
func NewOpenAI(baseURL, apiKey, model string, timeout time.Duration) *OpenAI {
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return &OpenAI{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		model:   model,
		http:    &http.Client{Timeout: timeout},
	}
}

type chatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
}

type chatResponse struct {
	Choices []struct {
		Message Message `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

// Complete implements Client.
func (c *OpenAI) Complete(ctx context.Context, messages []Message) (string, error) {
	body, err := json.Marshal(chatRequest{Model: c.model, Messages: messages})
	if err != nil {
		return "", fmt.Errorf("encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("call model: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	var parsed chatResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", fmt.Errorf("decode response (status %d): %w", resp.StatusCode, err)
	}
	if parsed.Error != nil {
		return "", fmt.Errorf("model error: %s", parsed.Error.Message)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("model returned status %d", resp.StatusCode)
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("model returned no choices")
	}
	return strings.TrimSpace(parsed.Choices[0].Message.Content), nil
}
