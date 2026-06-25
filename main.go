package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"logosserver/internal/api"
	"logosserver/internal/config"
	"logosserver/internal/db"
)

func main() {
	cfg := config.Load()

	ctx := context.Background()
	var store *db.Store
	if cfg.DatabaseURL == "" {
		log.Printf("DATABASE_URL is not set; healthcheck will pass, but API data endpoints return 503")
	} else {
		var err error
		store, err = db.Open(ctx, cfg.DatabaseURL)
		if err != nil {
			log.Printf("database unavailable: %v; healthcheck will pass, but API data endpoints return 503", err)
		} else {
			defer store.Close()

			if err := store.Migrate(ctx); err != nil {
				log.Printf("migrate database: %v; healthcheck will pass, but API data endpoints return 503", err)
				_ = store.Close()
				store = nil
			}
		}
	}

	router, err := api.NewRouter(cfg, store)
	if err != nil {
		log.Fatalf("router: %v", err)
	}

	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("server listening on :%s", cfg.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}
