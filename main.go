package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context) error {
	pool, err := openDatabase(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := migrateDatabase(ctx, pool); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", health)
	telegram, err := newTelegramBot(os.Getenv("TELEGRAM_OWNER_ID"), os.Getenv("TELEGRAM_BOT_TOKEN"))
	if err != nil {
		return err
	}
	go telegram.Start(ctx)
	server := &http.Server{
		Addr:              "0.0.0.0:" + port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
	}
	serverError := make(chan error, 1)
	go func() { serverError <- server.ListenAndServe() }()
	select {
	case err := <-serverError:
		return err
	case <-ctx.Done():
		return server.Close()
	}
}

func health(w http.ResponseWriter, _ *http.Request) {
	_, err := fmt.Fprintln(w, "ok")
	if err != nil {
		return
	}
}
