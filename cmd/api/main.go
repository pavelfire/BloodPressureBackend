package main

import (
	"log"
	"net/http"
	"strings"

	"bloodpressure/backend/internal/auth"
	"bloodpressure/backend/internal/config"
	"bloodpressure/backend/internal/db"
	"bloodpressure/backend/internal/migrate"
	"bloodpressure/backend/internal/readings"
)

func main() {
	cfg := config.Load()

	conn, err := db.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database connection failed: %v", err)
	}
	defer conn.Close()

	if err := migrate.Up(conn, "migrations"); err != nil {
		log.Fatalf("migrations failed: %v", err)
	}

	authService := auth.NewService(conn, cfg)
	authHandler := auth.NewHandler(authService)

	readingsRepo := readings.NewRepository(conn)
	readingsService := readings.NewService(readingsRepo)
	readingsHandler := readings.NewHandler(readingsService)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	mux.HandleFunc("POST /api/v1/auth/register", authHandler.Register)
	mux.HandleFunc("POST /api/v1/auth/login", authHandler.Login)
	mux.HandleFunc("POST /api/v1/auth/refresh", authHandler.Refresh)
	mux.HandleFunc("POST /api/v1/auth/logout", authHandler.Logout)

	protected := http.NewServeMux()
	protected.HandleFunc("GET /api/v1/auth/me", authHandler.Me)
	protected.HandleFunc("GET /api/v1/readings", readingsHandler.List)
	protected.HandleFunc("POST /api/v1/readings", readingsHandler.Create)
	protected.HandleFunc("POST /api/v1/sync", readingsHandler.Sync)
	protected.HandleFunc("/api/v1/readings/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			readingsHandler.Get(w, r)
		case http.MethodPut:
			readingsHandler.Update(w, r)
		case http.MethodDelete:
			readingsHandler.Delete(w, r)
		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.Handle("/api/v1/", auth.Middleware(authService)(protected))

	handler := auth.CORSMiddleware(cfg.CORSOrigin)(mux)
	addr := ":" + strings.TrimPrefix(cfg.Port, ":")
	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
