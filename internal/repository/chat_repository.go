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

func (r *ChatRepository) CreateSession(ctx context.Context, title string) (*entity.ChatSession, error) {
	s := entity.ChatSession{Title: title}
	err := r.db.QueryRow(ctx,
		`insert into chat_sessions (title) values ($1) returning id, created_at`, title,
	).Scan(&s.ID, &s.CreatedAt)
	return &s, err
}

func (r *ChatRepository) FindSession(ctx context.Context, id string) (*entity.ChatSession, error) {
	var s entity.ChatSession
	err := r.db.QueryRow(ctx,
		`select id, coalesce(title, ''), created_at from chat_sessions where id = $1`, id,
	).Scan(&s.ID, &s.Title, &s.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &s, err
}

func (r *ChatRepository) ListSessions(ctx context.Context) ([]entity.ChatSession, error) {
	rows, err := r.db.Query(ctx,
		`select id, coalesce(title, ''), created_at from chat_sessions order by created_at desc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sessions := []entity.ChatSession{}
	for rows.Next() {
		var s entity.ChatSession
		if err := rows.Scan(&s.ID, &s.Title, &s.CreatedAt); err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

// AddMessage menyimpan pesan; Sources otomatis di-encode pgx jadi JSONB (nil → NULL)
func (r *ChatRepository) AddMessage(ctx context.Context, m *entity.ChatMessage) error {
	return r.db.QueryRow(ctx, `
		insert into chat_messages (session_id, role, content, sources)
		values ($1, $2, $3, $4)
		returning id, created_at`,
		m.SessionID, m.Role, m.Content, m.Sources,
	).Scan(&m.ID, &m.CreatedAt)
}

func (r *ChatRepository) ListMessages(ctx context.Context, sessionID string) ([]entity.ChatMessage, error) {
	rows, err := r.db.Query(ctx, `
		select id, session_id, role, content, sources, created_at
		from chat_messages where session_id = $1
		order by created_at`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	defer rows.Close()

	messages := []entity.ChatMessage{}
	for rows.Next() {
		var m entity.ChatMessage
		if err := rows.Scan(&m.ID, &m.SessionID, &m.Role, &m.Content, &m.Sources, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}
