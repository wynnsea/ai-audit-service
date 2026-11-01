package config

import (
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"

)

type Config struct {
	Port        string
	DatabaseURL string
	AIBaseURL   string
	AIAPIKey    string
	AITimeout   time.Duration
	ChatModel   string
	EmbedModel  string
	SupabaseURL        string
	SupabaseServiceKey string
	StorageBucket      string
	EmbedBaseURL	   string
	RerankBaseURL	   string
	RerankModel	 	   string		
}


func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("no .env file found, using system env")
	}

	
	return &Config{
		Port:        getEnv("PORT", "8080"),
		DatabaseURL: mustEnv("DATABASE_URL"),
		AIBaseURL:   mustEnv("AI_BASE_URL"),
		AIAPIKey:    os.Getenv("AI_API_KEY"),
		ChatModel:   mustEnv("CHAT_MODEL"),
		AITimeout:   getDuration("AI_TIMEOUT", 60*time.Second),
		SupabaseURL: mustEnv("SUPABASE_URL"),
		SupabaseServiceKey: mustEnv("SUPABASE_SERVICE_KEY"),
		StorageBucket: getEnv("STORAGE_BUCKET", "documents"),
		EmbedBaseURL: mustEnv("EMBED_BASE_URL"),
		EmbedModel:  mustEnv("EMBED_MODEL"),
		RerankBaseURL:  mustEnv("RERANK_BASE_URL"),
		RerankModel:  mustEnv("RERANK_MODEL"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("env %s is required", key)
	}
	return v
}

func getDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		log.Fatalf("env %s: invalid duration %q", key, v)
	}
	return d
}
