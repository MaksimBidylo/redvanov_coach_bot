package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
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
	telegram, err := newTelegramBot(os.Getenv("TELEGRAM_OWNER_ID"), os.Getenv("TELEGRAM_BOT_TOKEN"), pool)
	if err != nil {
		return err
	}
	go telegram.Start(ctx)
	return serveHTTP(ctx)
}
