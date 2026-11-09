package extractor

import (
	"ai-audit-service/internal/entity"
	"context"
	"fmt"
)

func (e *Extractor) extractImage(ctx context.Context, data []byte, model string, mime string) ([]entity.Page, error) {
	text, err := e.ocr.TranscribeImage(ctx, model, data, mime)
	if err != nil {
		return nil, fmt.Errorf("ocr image: %w", err)
	}
	// Number 0: gambar tidak punya nomor halaman, jadi UI tidak menampilkan "hal."
	return []entity.Page{{Text: text, OCR: true}}, nil
}
