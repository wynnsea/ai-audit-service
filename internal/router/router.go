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
	mux.HandleFunc("GET /chat/sessions", chat.ListSessions)
	mux.HandleFunc("GET /chat/sessions/{id}/messages", chat.ListMessages)

	return logRequests(mux)
}
