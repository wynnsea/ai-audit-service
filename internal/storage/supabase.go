package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Supabase struct {
	baseURL string
	key     string
	bucket  string
	http    *http.Client
}

func NewSupabase(baseURL, key, bucket string) *Supabase {
	return &Supabase{
		baseURL: strings.TrimRight(baseURL, "/"),
		key:     key,
		bucket:  bucket,
		http:    &http.Client{Timeout: 60 * time.Second},
	}
}

func (s *Supabase) Upload(ctx context.Context, path, contentType string, data []byte) error {
	url := fmt.Sprintf("%s/storage/v1/object/%s/%s", s.baseURL, s.bucket, path)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)
	s.setAuth(req)

	resp, err := s.http.Do(req)
	if err != nil {
		return fmt.Errorf("storage upload: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("storage upload: status %d: %s", resp.StatusCode, body)
	}
	return nil
}

func (s *Supabase) setAuth(req *http.Request) {
	req.Header.Set("apikey", s.key)
	// key legacy service_role berbentuk JWT dan juga harus dikirim sebagai Bearer;
	// key baru (sb_secret_...) cukup lewat header apikey
	if strings.HasPrefix(s.key, "eyJ") {
		req.Header.Set("Authorization", "Bearer "+s.key)
	}
}
