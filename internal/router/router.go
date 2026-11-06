package router

import (
	"ai-audit-service/internal/controller"
	"net/http"

)

func New(health *controller.HealthController, docs *controller.DocumentController, chat controller.ChatController) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", health.Check)

	mux.HandleFunc("POST /documents", docs.Upload)
	mux.HandleFunc("GET /documents", docs.List)
	mux.HandleFunc("GET /documents/{id}", docs.Get)

	mux.HandleFunc("POST /chat", chat.Ask)
	mux.HandleFunc("GET /chat/models", chat.ListModels)
	mux.HandleFunc("GET /chat/sessions", chat.ListSessions)
	mux.HandleFunc("PATCH /chat/sessions/{id}", chat.RenameSession)
	mux.HandleFunc("DELETE /chat/sessions/{id}", chat.DeleteSession)
	mux.HandleFunc("GET /chat/sessions/{id}/messages", chat.ListMessages)

	return logRequests(mux)
}
