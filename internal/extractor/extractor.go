package extractor

import (
	"ai-audit-service/internal/entity"
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

type OCR interface {
	TranscribeImage(ctx context.Context, model string, img []byte, mime string) (string, error)
}

type Extractor struct {
	ocr OCR
}

func New(ocr OCR) *Extractor {
	return &Extractor{ocr: ocr}
}

func (e *Extractor) Extract(ctx context.Context, fileName string, data []byte, model string) ([]entity.Page, error) {
	switch strings.ToLower(filepath.Ext(fileName)) {
	case ".pdf":
		return e.extractPDF(ctx, data, model)
	case ".docx":
		return extractDOCX(data)
	case ".txt":
		return []entity.Page{{Text: string(data)}}, nil
	case ".xlsx":
		return extractXLSX(data)
	case ".png":
		return e.extractImage(ctx, data, model, "image/png")
	case ".jpg", ".jpeg":
		return e.extractImage(ctx, data, model, "image/jpeg")
	}

	return nil, fmt.Errorf("unsupported file: %s", fileName)
}
