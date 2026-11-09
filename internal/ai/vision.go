package ai

import (
	"context"
	"encoding/base64"
	"errors"
	"regexp"
	"strings"
)

const transcribePrompt = `Transcribe all text on this document page exactly as written, in its original language.
Do not translate, summarize, correct, or add anything.
Keep headings, clause numbers (e.g. 5.1, A.8.2, Pasal 3), and list structure, one item per line.
Write every table as a Markdown table: one table row per line, with cells separated by |, and keep empty cells empty. Never put each cell on its own line.
Write form fields as "Label : value" on one line.
Output plain text only: no HTML, no code blocks, no coordinates.
Output only the transcribed text, no commentary.`

var codeFence = regexp.MustCompile("(?m)^```.*$")

type contentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *imageURL `json:"image_url,omitempty"`
}

type imageURL struct {
	URL string `json:"url"`
}

type visionMessage struct {
	Role    string        `json:"role"`
	Content []contentPart `json:"content"`
}

type visionRequest struct {
	Model              string          `json:"model"`
	Messages           []visionMessage `json:"messages"`
	Temperature        float64         `json:"temperature"`
	MaxTokens          int             `json:"max_tokens"`
	ChatTemplateKwargs map[string]any  `json:"chat_template_kwargs,omitempty"`
}

// TranscribeImage dipakai sebagai OCR untuk halaman PDF hasil scan
func (c *Client) TranscribeImage(ctx context.Context, model string, img []byte, mime string) (string, error) {
	req := visionRequest{
		Model: model,
		Messages: []visionMessage{{
			Role: "user",
			Content: []contentPart{
				{Type: "text", Text: transcribePrompt},
				{Type: "image_url", ImageURL: &imageURL{
					URL: "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(img),
				}},
			},
		}},
		Temperature:        0,
		MaxTokens:          4096,
		ChatTemplateKwargs: map[string]any{"enable_thinking": false},
	}

	var resp chatResponse
	if err := c.post(ctx, c.cfg.ChatBaseURL+"/chat/completions", model, req, &resp); err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", errors.New("ai vision: empty choices")
	}

	text := thinkTag.ReplaceAllString(resp.Choices[0].Message.Content, "")
	text = codeFence.ReplaceAllString(text, "")
	
	return strings.TrimSpace(text), nil
}