package entity

import "time"

// SearchResult adalah chunk hasil vector search beserta info dokumennya
type SearchResult struct {
	Chunk
	ChunkID       string
	DocumentID    string
	DocumentTitle string
	Similarity    float64
}

// Source adalah kutipan dokumen yang dipakai untuk menjawab, nomornya dirujuk di jawaban sebagai [1], [2], ...
type Source struct {
	Number        int     `json:"number"`
	ChunkID       string  `json:"chunk_id"`
	DocumentID    string  `json:"document_id"`
	DocumentTitle string  `json:"document_title"`
	Section       string  `json:"section"`
	Page          int     `json:"page,omitempty"`
	Content       string  `json:"content"`
	Score         float64 `json:"score"`
}

const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
)

type ChatSession struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
}

type ChatMessage struct {
	ID        string    `json:"id"`
	SessionID string    `json:"session_id"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Sources   []Source  `json:"sources,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}
