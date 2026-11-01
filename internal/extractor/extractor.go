package extractor

import (
	"ai-audit-service/internal/entity"
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

type OCR interface {
	TranscribeImage(ctx context.Context, png []byte) (string, error)
}

type Extractor struct {
	ocr OCR
}

func New(ocr OCR) *Extractor {
	return &Extractor{ocr: ocr}
}

func (e *Extractor) Extract(ctx context.Context, fileName string, data []byte) ([]entity.Page, error) {
	switch strings.ToLower(filepath.Ext(fileName)) {
	case ".pdf":
		return e.extractPDF(ctx, data)
	case ".docx":
		return extractDOCX(data)
	case ".txt":
		return []entity.Page{{Text: string(data)}}, nil
	}
	return nil, fmt.Errorf("unsupported file: %s", fileName)
}
