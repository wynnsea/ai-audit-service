package ai

import (
	"context"
	"fmt"

)

type rerankRequest struct {
	Model     string   `json:"model"`
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
}

type rerankResponse struct {
	Results []struct {
		Index          int     `json:"index"`
		RelevanceScore float64 `json:"relevance_score"`
	} `json:"results"`
}

// Rerank menilai relevansi tiap dokumen terhadap query (0 = tidak relevan, 1 = sangat relevan).
// Skor dikembalikan sesuai urutan input documents.
func (c *Client) Rerank(ctx context.Context, query string, documents []string) ([]float64, error) {
	var resp rerankResponse
	req := rerankRequest{Model: c.cfg.RerankModel, Query: query, Documents: documents}
	if err := c.post(ctx, c.cfg.RerankBaseURL+"/rerank", c.cfg.RerankModel, req, &resp); err != nil {
		return nil, err
	}

	// response diurutkan dari skor tertinggi, jadi dikembalikan ke urutan input pakai index
	scores := make([]float64, len(documents))
	for _, r := range resp.Results {
		if r.Index < 0 || r.Index >= len(documents) {
			return nil, fmt.Errorf("rerank: invalid index %d", r.Index)
		}
		scores[r.Index] = r.RelevanceScore
	}
	return scores, nil
}