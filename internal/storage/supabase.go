package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ErrNotFound: file tidak ada di bucket (misal sudah dihapus lewat dashboard)
var ErrNotFound = errors.New("storage: file not found")

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

// Download membuka file dari bucket. Body dikirim bertahap (stream), jadi file besar tidak
// perlu dimuat utuh ke memori; pemanggil wajib menutup reader-nya.
func (s *Supabase) Download(ctx context.Context, path string) (io.ReadCloser, int64, error) {
	url := fmt.Sprintf("%s/storage/v1/object/%s/%s", s.baseURL, s.bucket, path)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("build request: %w", err)
	}
	s.setAuth(req)

	resp, err := s.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("storage download: %w", err)
	}
	if resp.StatusCode >= 300 {
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		// Supabase kadang membalas 400 dengan "not_found", bukan 404
		if resp.StatusCode == http.StatusNotFound || bytes.Contains(body, []byte("not_found")) {
			return nil, 0, ErrNotFound
		}
		return nil, 0, fmt.Errorf("storage download: status %d: %s", resp.StatusCode, body)
	}
	return resp.Body, resp.ContentLength, nil
}

// Delete menghapus file dari bucket. File yang memang sudah tidak ada dianggap berhasil.
func (s *Supabase) Delete(ctx context.Context, path string) error {
	url := fmt.Sprintf("%s/storage/v1/object/%s/%s", s.baseURL, s.bucket, path)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	s.setAuth(req)

	resp, err := s.http.Do(req)
	if err != nil {
		return fmt.Errorf("storage delete: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		if resp.StatusCode == http.StatusNotFound || bytes.Contains(body, []byte("not_found")) {
			return nil
		}
		return fmt.Errorf("storage delete: status %d: %s", resp.StatusCode, body)
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
