package usecase

import (
	"ai-audit-service/internal/chunker"
	"ai-audit-service/internal/entity"
	"context"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

)

var ErrUnsupportedFile = errors.New("unsupported file type, allowed: .pdf, .docx, .txt")

var allowedTypes = map[string]string{
	".pdf":  "application/pdf",
	".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	".txt":  "text/plain",
}

type DocumentRepository interface {
	Create(ctx context.Context, doc *entity.Document) error
	FindByID(ctx context.Context, id string) (*entity.Document, error)
	List(ctx context.Context) ([]entity.Document, error)
	UpdateStatus(ctx context.Context, id, status, errMsg string) error
}

type FileStorage interface {
	Upload(ctx context.Context, path, contentType string, data []byte) error
}

type UploadInput struct {
	Title          string
	RegulationType string
	FileName       string
	Content        []byte
}

type TextExtractor interface {
	Extract(ctx context.Context, fileName string, data []byte) ([]entity.Page, error)
}

type ChunkRepository interface {
	ReplaceForDocument(ctx context.Context, documentID string, chunk []entity.Chunk) error
}

type DocumentUsecase struct {
	repo      DocumentRepository
	storage   FileStorage
	extractor TextExtractor
	chunks    ChunkRepository
	embedder  Embedder
}

type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

func NewDocumentUsecase(repo DocumentRepository, storage FileStorage, extractor TextExtractor, chunks ChunkRepository, embedder Embedder) *DocumentUsecase {
	return &DocumentUsecase{repo: repo, storage: storage, extractor: extractor, chunks: chunks, embedder: embedder}
}

func (u *DocumentUsecase) Upload(ctx context.Context, in UploadInput) (*entity.Document, error) {
	ext := strings.ToLower(filepath.Ext(in.FileName))
	contentType, ok := allowedTypes[ext]
	if !ok {
		return nil, ErrUnsupportedFile
	}

	id := uuid.NewString()
	title := strings.TrimSpace(in.Title)
	if title == "" {
		title = strings.TrimSuffix(in.FileName, filepath.Ext(in.FileName))
	}

	doc := &entity.Document{
		ID:             id,
		Title:          title,
		FileName:       in.FileName,
		StoragePath:    id + ext, // pakai ID biar nama file aneh/duplikat nggak masalah
		RegulationType: strings.TrimSpace(in.RegulationType),
		Status:         entity.DocumentStatusProcessing,
	}

	if err := u.storage.Upload(ctx, doc.StoragePath, contentType, in.Content); err != nil {
		return nil, fmt.Errorf("upload file: %w", err)
	}
	if err := u.repo.Create(ctx, doc); err != nil {
		return nil, fmt.Errorf("save document: %w", err)
	}

	go u.process(doc, in.Content)

	return doc, nil
}

func (u *DocumentUsecase) Get(ctx context.Context, id string) (*entity.Document, error) {
	return u.repo.FindByID(ctx, id)
}

func (u *DocumentUsecase) List(ctx context.Context) ([]entity.Document, error) {
	return u.repo.List(ctx)
}

const ingestTimeout = 15 * time.Minute

func (u *DocumentUsecase) process(doc *entity.Document, content []byte) {
	// goroutine panic bakal matiin seluruh server, jadi ditangkap di sini
	defer func() {
		if r := recover(); r != nil {
			u.fail(doc.ID, fmt.Errorf("panic: %v", r))
		}
	}()

	// context baru: request HTTP-nya udah selesai waktu ini jalan
	ctx, cancel := context.WithTimeout(context.Background(), ingestTimeout)
	defer cancel()

	pages, err := u.extractor.Extract(ctx, doc.FileName, content)
	if err != nil {
		u.fail(doc.ID, fmt.Errorf("extract text: %w", err))
		return
	}

	ocrCount := 0
	for _, p := range pages {
		if p.OCR {
			ocrCount++
		}
	}
	log.Printf("document %s: extracted %d pages (%d via OCR)", doc.ID, len(pages), ocrCount)

	chunks := chunker.Split(pages)
	if len(chunks) == 0 {
		u.fail(doc.ID, errors.New("no text could be extracted from document"))
		return
	}
	
	log.Printf("document %s: %d pages (%d via OCR) -> %d chunks", doc.ID, len(pages), ocrCount, len(chunks))

	if err := u.embedChunks(ctx, chunks); err != nil {
		u.fail(doc.ID, fmt.Errorf("embed chunks: %w", err))
		return
	}

	if err := u.chunks.ReplaceForDocument(ctx, doc.ID, chunks); err != nil {
		u.fail(doc.ID, fmt.Errorf("save chunks: %w", err))
		return
	}

	if err := u.repo.UpdateStatus(ctx, doc.ID, entity.DocumentStatusReady, ""); err != nil {
		log.Printf("document %s: update status: %v", doc.ID, err)
		return
	}
	log.Printf("document %s: ready", doc.ID)
}

func (u *DocumentUsecase) fail(id string, cause error) {
	log.Printf("document %s: ingest failed: %v", id, cause)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := u.repo.UpdateStatus(ctx, id, entity.DocumentStatusFailed, cause.Error()); err != nil {
		log.Printf("document %s: update status: %v", id, err)
	}
}

func (u *DocumentUsecase) embedChunks(ctx context.Context, chunks []entity.Chunk) error {
	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = EmbeddingText(c)
	}

	vectors, err := u.embedder.Embed(ctx, texts)
	if err != nil {
		return err
	}
	for i := range chunks {
		chunks[i].Embedding = vectors[i]
	}
	return nil
}

// EmbeddingText menyertakan section supaya posisi chunk ("BAB VI > Pasal 40") ikut terbaca model
func EmbeddingText(c entity.Chunk) string {
	return c.Section + "\n" + c.Content
}
