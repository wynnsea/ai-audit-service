package controller

import (
	"ai-audit-service/internal/pkg/response"
	"ai-audit-service/internal/usecase"
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/google/uuid"
)

const maxChatBody = 64 << 10 // 64 KB

type ChatController struct {
	uc *usecase.ChatUsecase
}

func NewChatController(uc *usecase.ChatUsecase) *ChatController {
	return &ChatController{uc: uc}
}

type askRequest struct {
	SessionID string `json:"session_id"`
	Question  string `json:"question"`
	Model     string `json:"model"`
	// kosong = semua dokumen
	DocumentIDs []string `json:"document_ids"`
}

func (c *ChatController) Ask(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxChatBody)
	var req askRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.SessionID != "" {
		if _, err := uuid.Parse(req.SessionID); err != nil {
			response.Error(w, http.StatusNotFound, "chat session not found")
			return
		}
	}

	for _, id := range req.DocumentIDs {
		if _, err := uuid.Parse(id); err != nil {
			response.Error(w, http.StatusBadRequest, "invalid document_ids")
			return
		}
	}

	out, err := c.uc.Ask(r.Context(), usecase.AskInput{SessionID: req.SessionID, Question: req.Question, Model: req.Model, DocumentIDs: req.DocumentIDs})
	switch {
	case errors.Is(err, usecase.ErrEmptyQuestion), errors.Is(err, usecase.ErrUnknownModel):
		response.Error(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, usecase.ErrSessionNotFound):
		response.Error(w, http.StatusNotFound, err.Error())
	case err != nil:
		log.Printf("chat ask: %v", err)
		response.Error(w, http.StatusInternalServerError, "internal error")
	default:
		response.JSON(w, http.StatusOK, out)
	}
}

func (c *ChatController) ListModels(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, http.StatusOK, c.uc.ChatModels())
}

func (c *ChatController) ListSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := c.uc.ListSessions(r.Context())
	if err != nil {
		log.Printf("list sessions: %v", err)
		response.Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	response.JSON(w, http.StatusOK, sessions)
}

type renameRequest struct {
	Title string `json:"title"`
}

func (c *ChatController) RenameSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		response.Error(w, http.StatusNotFound, "chat session not found")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxChatBody)
	var req renameRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	title, err := c.uc.RenameSession(r.Context(), id, req.Title)
	switch {
	case errors.Is(err, usecase.ErrEmptyTitle):
		response.Error(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, usecase.ErrSessionNotFound):
		response.Error(w, http.StatusNotFound, err.Error())
	case err != nil:
		log.Printf("rename session: %v", err)
		response.Error(w, http.StatusInternalServerError, "internal error")
	default:
		response.JSON(w, http.StatusOK, map[string]string{"id": id, "title": title})
	}
}

func (c *ChatController) DeleteSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		response.Error(w, http.StatusNotFound, "chat session not found")
		return
	}

	err := c.uc.DeleteSession(r.Context(), id)
	switch {
	case errors.Is(err, usecase.ErrSessionNotFound):
		response.Error(w, http.StatusNotFound, err.Error())
	case err != nil:
		log.Printf("delete session: %v", err)
		response.Error(w, http.StatusInternalServerError, "internal error")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (c *ChatController) ListMessages(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		response.Error(w, http.StatusNotFound, "chat session not found")
		return
	}

	messages, err := c.uc.ListMessages(r.Context(), id)
	switch {
	case errors.Is(err, usecase.ErrSessionNotFound):
		response.Error(w, http.StatusNotFound, err.Error())
	case err != nil:
		log.Printf("list messages: %v", err)
		response.Error(w, http.StatusInternalServerError, "internal error")
	default:
		response.JSON(w, http.StatusOK, messages)
	}
}
