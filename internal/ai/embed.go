package ai

import (
	"context"
	"fmt"
)

// jumlah teks per request; terlalu besar bikin request lambat/timeout
const embedBatchSize = 32

type embedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type embedResponse struct {
	Data []struct {
		Index     int       `json:"index"`
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

// Embed mengubah teks jadi vector. Input dipecah per batch secara otomatis.
func (c *Client) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	vectors := make([][]float32, 0, len(texts))
	for start := 0; start < len(texts); start += embedBatchSize {
		batch := texts[start:min(start+embedBatchSize, len(texts))]

		vecs, err := c.embedBatch(ctx, batch)
		if err != nil {
			return nil, fmt.Errorf("embed batch %d: %w", start/embedBatchSize, err)
		}
		vectors = append(vectors, vecs...)
	}
	return vectors, nil
}

func (c *Client) embedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	var resp embedResponse
	req := embedRequest{Model: c.cfg.EmbedModel, Input: texts}
	if err := c.post(ctx, c.cfg.EmbedBaseURL+"/embeddings", c.cfg.EmbedModel, req, &resp); err != nil {
		return nil, err
	}
	if len(resp.Data) != len(texts) {
		return nil, fmt.Errorf("got %d vectors for %d inputs", len(resp.Data), len(texts))
	}

	// urutan response belum tentu sama dengan input, jadi disusun ulang pakai index
	vectors := make([][]float32, len(texts))
	for _, d := range resp.Data {
		if d.Index < 0 || d.Index >= len(texts) {
			return nil, fmt.Errorf("invalid index %d", d.Index)
		}
		vectors[d.Index] = d.Embedding
	}
	return vectors, nil
}
