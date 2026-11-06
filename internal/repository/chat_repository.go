package repository

import (
	"ai-audit-service/internal/entity"
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

)

type ChatRepository struct {
	db *pgxpool.Pool
}

func NewChatRepository(db *pgxpool.Pool) *ChatRepository {
	return &ChatRepository{db: db}
}

func (r *ChatRepository) CreateSession(ctx context.Context, title string, documentIDs []string) (*entity.ChatSession, error) {
	s := entity.ChatSession{Title: title, DocumentIDs: documentIDs}
	err := r.db.QueryRow(ctx,
		`insert into chat_sessions (title, document_ids) values ($1, $2::uuid[]) returning id, created_at, last_message_at`, title, documentIDs,
	).Scan(&s.ID, &s.CreatedAt, &s.LastMessageAt)
	return &s, err
}

// SetSessionDocuments mengganti dokumen yang boleh dipakai sesi; nil = semua dokumen
func (r *ChatRepository) SetSessionDocuments(ctx context.Context, id string, documentIDs []string) error {
	_, err := r.db.Exec(ctx, `update chat_sessions set document_ids = $2::uuid[] where id = $1`, id, documentIDs)
	return err
}

func (r *ChatRepository) FindSession(ctx context.Context, id string) (*entity.ChatSession, error) {
	var s entity.ChatSession
	err := r.db.QueryRow(ctx,
		`select id, coalesce(title, ''), document_ids, created_at, last_message_at from chat_sessions where id = $1`, id,
	).Scan(&s.ID, &s.Title, &s.DocumentIDs, &s.CreatedAt, &s.LastMessageAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &s, err
}

// RenameSession mengganti judul sesi; urutan riwayat tidak berubah
func (r *ChatRepository) RenameSession(ctx context.Context, id, title string) error {
	tag, err := r.db.Exec(ctx, `update chat_sessions set title = $2 where id = $1`, id, title)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteSession menghapus sesi; pesannya ikut terhapus lewat on delete cascade
func (r *ChatRepository) DeleteSession(ctx context.Context, id string) error {
	tag, err := r.db.Exec(ctx, `delete from chat_sessions where id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// TouchSession menandai sesi baru dipakai, supaya naik ke atas riwayat
func (r *ChatRepository) TouchSession(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `update chat_sessions set last_message_at = now() where id = $1`, id)
	return err
}

func (r *ChatRepository) ListSessions(ctx context.Context) ([]entity.ChatSession, error) {
	rows, err := r.db.Query(ctx,
		`select id, coalesce(title, ''), document_ids, created_at, last_message_at
		from chat_sessions order by last_message_at desc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sessions := []entity.ChatSession{}
	for rows.Next() {
		var s entity.ChatSession
		if err := rows.Scan(&s.ID, &s.Title, &s.DocumentIDs, &s.CreatedAt, &s.LastMessageAt); err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

// AddMessage menyimpan pesan; Sources otomatis di-encode pgx jadi JSONB (nil → NULL)
func (r *ChatRepository) AddMessage(ctx context.Context, m *entity.ChatMessage) error {
	return r.db.QueryRow(ctx, `
		insert into chat_messages (session_id, role, content, sources, model)
		values ($1, $2, $3, $4, nullif($5, ''))
		returning id, created_at`,
		m.SessionID, m.Role, m.Content, m.Sources, m.Model,
	).Scan(&m.ID, &m.CreatedAt)
}

func (r *ChatRepository) ListMessages(ctx context.Context, sessionID string) ([]entity.ChatMessage, error) {
	rows, err := r.db.Query(ctx, `
		select id, session_id, role, content, sources, coalesce(model, ''), created_at
		from chat_messages where session_id = $1
		order by created_at`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	defer rows.Close()

	messages := []entity.ChatMessage{}
	for rows.Next() {
		var m entity.ChatMessage
		if err := rows.Scan(&m.ID, &m.SessionID, &m.Role, &m.Content, &m.Sources, &m.Model, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}
