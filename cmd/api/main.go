package main

import (
	"ai-audit-service/internal/ai"
	"ai-audit-service/internal/config"
	"ai-audit-service/internal/controller"
	"ai-audit-service/internal/database"
	"ai-audit-service/internal/extractor"
	"ai-audit-service/internal/repository"
	"ai-audit-service/internal/router"
	"ai-audit-service/internal/storage"
	"ai-audit-service/internal/usecase"
	"context"
	"errors"
	"log"
	"net/http"
	"time"

)

func main() {
	cfg := config.Load()

	db, err := database.NewPostgres(context.Background(), cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	aiClient := ai.NewClient(ai.Config{
		ChatBaseURL:   cfg.AIBaseURL,
		EmbedBaseURL:  cfg.EmbedBaseURL,
		APIKey:        cfg.AIAPIKey,
		ChatModel:     cfg.ChatModel,
		EmbedModel:    cfg.EmbedModel,
		Timeout:       cfg.AITimeout,
		RerankBaseURL: cfg.RerankBaseURL,
		RerankModel:   cfg.RerankModel,
	})
	ext := extractor.New(aiClient)
	chunk := repository.NewChunkRepository(db)

	store := storage.NewSupabase(cfg.SupabaseURL, cfg.SupabaseServiceKey, cfg.StorageBucket)
	docRepo := repository.NewDocumentRepository(db)
	docUC := usecase.NewDocumentUsecase(docRepo, store, ext, chunk, aiClient)

	healthCtrl := controller.NewHealthController(db)
	docCtrl := controller.NewDocumentController(docUC)
	chatRepo := repository.NewChatRepository(db)
	chatUc := usecase.NewChatUsecase(chatRepo, chunk, aiClient, aiClient, aiClient) // aiClient = embedder, reranker, LLM
	chatCtrl := controller.NewChatController(chatUc)

	handler := router.New(healthCtrl, docCtrl, *chatCtrl)

	

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("listening on :%s", cfg.Port)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}