package ai

import (
	"context"
	"errors"
	"regexp"
	"strings"

)

type Message struct {
	Role    string `json:"role"` // system | user | assistant
	Content string `json:"content"`
}

type chatRequest struct {
	Model              string         `json:"model"`
	Messages           []Message      `json:"messages"`
	Temperature        float64        `json:"temperature"`
	ChatTemplateKwargs map[string]any `json:"chat_template_kwargs,omitempty"`
}


type chatResponse struct {
	Choices []struct {
		Message struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
		} `json:"message"`
	} `json:"choices"`
}

// model reasoning kadang nyelipin proses mikirnya di dalam content
var thinkTag = regexp.MustCompile(`(?s)<think>.*?</think>`)

func (c *Client) Chat(ctx context.Context, messages []Message) (string, error) {
	var resp chatResponse
		req := chatRequest{
			Model:    c.cfg.ChatModel,
			Messages: messages,
			Temperature: 0.1,
			ChatTemplateKwargs: map[string]any{"enable_thinking": false},
		}

	if err := c.post(ctx, c.cfg.ChatBaseURL+"/chat/completions", req, &resp); err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", errors.New("ai chat: empty choices")
	}

	answer := thinkTag.ReplaceAllString(resp.Choices[0].Message.Content, "")
	return strings.TrimSpace(answer), nil
}