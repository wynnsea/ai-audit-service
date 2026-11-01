package repository

import (
	"ai-audit-service/internal/entity"
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

)

type ChunkRepository struct {
	db *pgxpool.Pool
}

func NewChunkRepository(db *pgxpool.Pool) *ChunkRepository {
	return &ChunkRepository{db: db}
}

// ReplaceForDocument menghapus chunk lama dokumen lalu menyimpan yang baru dalam satu transaksi,
// jadi kalau dokumen diproses ulang chunk-nya nggak dobel dan ga ke simpan setengah
func (r *ChunkRepository) ReplaceForDocument(ctx context.Context, documentID string, chunks []entity.Chunk) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) // no-op kalau sudah commit

	if _, err := tx.Exec(ctx, `delete from document_chunks where document_id = $1`, documentID); err != nil {
		return fmt.Errorf("delete old chunks: %w", err)
	}

	// batch = semua insert dikirim dalam satu round trip ke database
	batch := &pgx.Batch{}
	for _, c := range chunks {
		var embedding any // nil → NULL kalau chunk belum punya embedding
		if len(c.Embedding) > 0 {
			embedding = vectorLiteral(c.Embedding)
		}
		batch.Queue(`
			insert into document_chunks (document_id, chunk_index, section, page, content, embedding)
			values ($1, $2, nullif($3, ''), nullif($4, 0), $5, $6::text::halfvec)`,
			documentID, c.Index, c.Section, c.Page, c.Content, embedding)
	}

	if err := tx.SendBatch(ctx, batch).Close(); err != nil {
		return fmt.Errorf("insert chunks: %w", err)
	}

	return tx.Commit(ctx)
}

// kolom yang sama untuk semua query pencarian; scanSearchResults membacanya dengan urutan ini
const searchColumns = `c.id, c.document_id, d.title, coalesce(c.section, ''), coalesce(c.page, 0), c.content`

// nomor Pasal dari section, contoh "BAB VI ... > Pasal 40 ayat (2)" → "40"
const sectionPasal = `substring(c.section from '(?:^|> )Pasal ([0-9]+[A-Z]?)')`

// SearchSimilar mencari chunk dengan embedding paling mirip (cosine) dari dokumen yang sudah ready
func (r *ChunkRepository) SearchSimilar(ctx context.Context, embedding []float32, limit int) ([]entity.SearchResult, error) {
	// <=> = cosine distance (0 = identik), jadi similarity = 1 - distance
	return r.search(ctx, `
		select `+searchColumns+`, 1 - (c.embedding <=> $1::text::halfvec)
		from document_chunks c
		join documents d on d.id = c.document_id
		where d.status = 'ready' and c.embedding is not null
		order by c.embedding <=> $1::text::halfvec
		limit $2`,
		vectorLiteral(embedding), limit)
}

// FindByPasal mengambil chunk batang tubuh (bukan Penjelasan) untuk nomor Pasal tertentu dalam satu dokumen
func (r *ChunkRepository) FindByPasal(ctx context.Context, documentID string, numbers []string) ([]entity.SearchResult, error) {
	return r.search(ctx, `
		select `+searchColumns+`, 0
		from document_chunks c
		join documents d on d.id = c.document_id
		where c.document_id = $1 and c.section !~ '^Penjelasan'
		  and `+sectionPasal+` = any($2)
		order by c.chunk_index`,
		documentID, numbers)
}

// FindReferencing mengambil chunk yang isinya menyebut "Pasal <number>" dalam satu dokumen
func (r *ChunkRepository) FindReferencing(ctx context.Context, documentID, number string) ([]entity.SearchResult, error) {
	// \m dan \M = batas kata di regex Postgres, jadi "Pasal 3" tidak cocok dengan "Pasal 32"
	return r.search(ctx, `
		select `+searchColumns+`, 0
		from document_chunks c
		join documents d on d.id = c.document_id
		where c.document_id = $1 and c.section !~ '^Penjelasan'
		  and c.content ~ ('\mPasal ' || $2 || '\M')
		order by c.chunk_index`,
		documentID, number)
}

func (r *ChunkRepository) search(ctx context.Context, sql string, args ...any) ([]entity.SearchResult, error) {
	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("search chunks: %w", err)
	}
	defer rows.Close()

	var results []entity.SearchResult
	for rows.Next() {
		var s entity.SearchResult
		if err := rows.Scan(&s.ChunkID, &s.DocumentID, &s.DocumentTitle, &s.Section, &s.Page,
			&s.Content, &s.Similarity); err != nil {
			return nil, fmt.Errorf("scan chunk: %w", err)
		}
		results = append(results, s)
	}
	return results, rows.Err()
}