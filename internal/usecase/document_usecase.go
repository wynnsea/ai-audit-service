package usecase

import (
	"ai-audit-service/internal/chunker"
	"ai-audit-service/internal/entity"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

var ErrUnsupportedFile = errors.New("unsupported file type, allowed: .pdf, .docx, .txt, .xlsx, .png, .jpg, .jpeg")
var ErrUnknownOCRModel = errors.New("ocr model is not available")

var allowedTypes = map[string]string{
	".pdf":  "application/pdf",
	".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	".txt":  "text/plain",
	".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
}

type DocumentRepository interface {
	Create(ctx context.Context, doc *entity.Document) error
	FindByID(ctx context.Context, id string) (*entity.Document, error)
	List(ctx context.Context) ([]entity.Document, error)
	UpdateStatus(ctx context.Context, id, status, errMsg string) error
	SetOCRModel(ctx context.Context, id, model string) error
	Delete(ctx context.Context, id string) error
}

type FileStorage interface {
	Upload(ctx context.Context, path, contentType string, data []byte) error
	Download(ctx context.Context, path string) (io.ReadCloser, int64, error)
	Delete(ctx context.Context, path string) error
}

// DocumentFile adalah file asli dokumen; Body wajib ditutup pemanggil
type DocumentFile struct {
	Name        string
	ContentType string
	Size        int64 // -1 kalau tidak diketahui
	Body        io.ReadCloser
}

type UploadInput struct {
	Title          string
	RegulationType string
	FileName       string
	Content        []byte
	OCRModel       string
}

type TextExtractor interface {
	Extract(ctx context.Context, fileName string, data []byte, model string) ([]entity.Page, error)
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
	ocrModels []string
	// slot proses ingest; upload banyak file sekaligus antre di sini, bukan jalan semua bersamaan
	ingestSlots chan struct{}
}

// jumlah dokumen yang diproses (OCR + embedding) bersamaan. Upload 50 CV sekaligus tanpa batas ini
// akan mengirim ratusan request ke gateway dalam waktu yang sama dan memicu timeout.
const maxConcurrentIngest = 3

type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

func NewDocumentUsecase(repo DocumentRepository, storage FileStorage, extractor TextExtractor, chunks ChunkRepository, embedder Embedder, ocrModels []string) *DocumentUsecase {
	return &DocumentUsecase{
		repo: repo, storage: storage, extractor: extractor, chunks: chunks, embedder: embedder,
		ocrModels: ocrModels, ingestSlots: make(chan struct{}, maxConcurrentIngest),
	}
}

func (u *DocumentUsecase) Upload(ctx context.Context, in UploadInput) (*entity.Document, error) {
	ext := strings.ToLower(filepath.Ext(in.FileName))
	contentType, ok := allowedTypes[ext]
	if !ok {
		return nil, ErrUnsupportedFile
	}

	ocrModel := in.OCRModel
	if ocrModel == "" {
		ocrModel = u.ocrModels[0]
	}
	// daftar ini mencegah user memanggil model lain yang ada di gateway
	if !slices.Contains(u.ocrModels, ocrModel) {
		return nil, ErrUnknownOCRModel
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

	go u.process(doc, in.Content, ocrModel)

	return doc, nil
}

// urutan pertama = model default
func (u *DocumentUsecase) OCRModels() []string {
	return u.ocrModels
}

// OpenFile mengambil file asli dari storage. Content-Type diambil dari daftar format yang diizinkan,
// bukan dari storage, supaya browser hanya menerima tipe yang memang kita kenal.
func (u *DocumentUsecase) OpenFile(ctx context.Context, id string) (*DocumentFile, error) {
	doc, err := u.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	contentType, ok := allowedTypes[strings.ToLower(filepath.Ext(doc.StoragePath))]
	if !ok {
		contentType = "application/octet-stream"
	}

	body, size, err := u.storage.Download(ctx, doc.StoragePath)
	if err != nil {
		return nil, err
	}
	return &DocumentFile{Name: doc.FileName, ContentType: contentType, Size: size, Body: body}, nil
}

// Delete menghapus dokumen beserta chunk dan file aslinya. Database dihapus duluan karena itu
// sumber kebenarannya: kalau file gagal dihapus, dokumen tetap hilang dari aplikasi dan
// sisa file cukup dicatat di log.
func (u *DocumentUsecase) Delete(ctx context.Context, id string) error {
	doc, err := u.repo.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if err := u.repo.Delete(ctx, id); err != nil {
		return err
	}
	if err := u.storage.Delete(ctx, doc.StoragePath); err != nil {
		log.Printf("document %s: deleted, but storage file %s remains: %v", id, doc.StoragePath, err)
	}
	return nil
}

func (u *DocumentUsecase) Get(ctx context.Context, id string) (*entity.Document, error) {
	return u.repo.FindByID(ctx, id)
}

func (u *DocumentUsecase) List(ctx context.Context) ([]entity.Document, error) {
	return u.repo.List(ctx)
}

const ingestTimeout = 15 * time.Minute

func (u *DocumentUsecase) process(doc *entity.Document, content []byte, ocrModel string) {
	// goroutine panic bakal matiin seluruh server, jadi ditangkap di sini
	defer func() {
		if r := recover(); r != nil {
			u.fail(doc.ID, fmt.Errorf("panic: %v", r))
		}
	}()

	// menunggu giliran; selama antre status dokumen tetap "processing"
	u.ingestSlots <- struct{}{}
	defer func() { <-u.ingestSlots }()

	// context baru: request HTTP-nya udah selesai waktu ini jalan.
	// Dibuat setelah dapat slot, supaya waktu antre tidak memakan jatah timeout.
	ctx, cancel := context.WithTimeout(context.Background(), ingestTimeout)
	defer cancel()

	pages, err := u.extractor.Extract(ctx, doc.FileName, content, ocrModel)
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
	log.Printf("document %s: extracted %d pages (%d via OCR, model=%q)", doc.ID, len(pages), ocrCount, ocrModel)
	// DOCX/TXT/XLSX dan PDF ber-teks tidak memanggil model OCR, jadi modelnya tidak dicatat
	if ocrCount > 0 {
		if err := u.repo.SetOCRModel(ctx, doc.ID, ocrModel); err != nil {
			log.Printf("document %s: save ocr model: %v", doc.ID, err)
		}
	}

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
