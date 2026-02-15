package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/innomon/adk2goose/internal/config"
	"github.com/innomon/adk2goose/internal/gooseclient"
	"github.com/innomon/adk2goose/internal/proxy"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	gooseClient := gooseclient.New(cfg.GooseBaseURL, cfg.GooseSecret)
	sessionMgr := proxy.NewSessionManager(gooseClient, cfg.WorkingDir)
	handler := proxy.NewHandler(sessionMgr, gooseClient)

	srv := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: cfg.RequestTimeout + 10*time.Second, // extra buffer for streaming
	}

	// Graceful shutdown on SIGINT/SIGTERM
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("shutting down...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	}()

	log.Printf("adk2goose proxy listening on %s → %s", cfg.ListenAddr, cfg.GooseBaseURL)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}
