package repository

import (
	"ai-audit-service/internal/entity"
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("not found")

type DocumentRepository struct {
	db *pgxpool.Pool
}

func NewDocumentRepository(db *pgxpool.Pool) *DocumentRepository {
	return &DocumentRepository{db: db}
}

func (r *DocumentRepository) Create(ctx context.Context, doc *entity.Document) error {
	const q = `
		insert into documents (id, title, file_name, storage_path, regulation_type, status)
		values ($1, $2, $3, $4, nullif($5, ''), $6)
		returning created_at`

	return r.db.QueryRow(ctx, q,
		doc.ID, doc.Title, doc.FileName, doc.StoragePath, doc.RegulationType, doc.Status,
	).Scan(&doc.CreatedAt)
}

const selectDocument = `
	select id, title, file_name, storage_path, coalesce(regulation_type, ''),
	       coalesce(ocr_model, ''), status, coalesce(error_message, ''), created_at
	from documents`

func scanDocument(row pgx.Row) (*entity.Document, error) {
	var d entity.Document
	err := row.Scan(&d.ID, &d.Title, &d.FileName, &d.StoragePath, &d.RegulationType,
		&d.OCRModel, &d.Status, &d.ErrorMessage, &d.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *DocumentRepository) FindByID(ctx context.Context, id string) (*entity.Document, error) {
	doc, err := scanDocument(r.db.QueryRow(ctx, selectDocument+` where id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return doc, err
}

func (r *DocumentRepository) List(ctx context.Context) ([]entity.Document, error) {
	rows, err := r.db.Query(ctx, selectDocument+` order by created_at desc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	docs := []entity.Document{}
	for rows.Next() {
		doc, err := scanDocument(rows)
		if err != nil {
			return nil, err
		}
		docs = append(docs, *doc)
	}
	return docs, rows.Err()
}

// SetOCRModel mencatat model yang membaca halaman scan/gambar, supaya hasil OCR yang jelek bisa ditelusuri
func (r *DocumentRepository) SetOCRModel(ctx context.Context, id, model string) error {
	_, err := r.db.Exec(ctx, `update documents set ocr_model = $2 where id = $1`, id, model)
	return err
}

// Delete menghapus dokumen; chunk-nya ikut terhapus lewat on delete cascade
func (r *DocumentRepository) Delete(ctx context.Context, id string) error {
	tag, err := r.db.Exec(ctx, `delete from documents where id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *DocumentRepository) UpdateStatus(ctx context.Context, id, status, errMsg string) error {
	_, err := r.db.Exec(ctx,
		`update documents set status = $2, error_message = nullif($3, '') where id = $1`,
		id, status, errMsg)
	return err
}
