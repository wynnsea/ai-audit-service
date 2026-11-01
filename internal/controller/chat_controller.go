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

	out, err := c.uc.Ask(r.Context(), usecase.AskInput{SessionID: req.SessionID, Question: req.Question})
	switch {
	case errors.Is(err, usecase.ErrEmptyQuestion):
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

func (c *ChatController) ListSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := c.uc.ListSessions(r.Context())
	if err != nil {
		log.Printf("list sessions: %v", err)
		response.Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	response.JSON(w, http.StatusOK, sessions)
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
