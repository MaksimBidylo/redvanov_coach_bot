package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", health)
	telegram, err := newTelegramBot(os.Getenv("TELEGRAM_OWNER_ID"), os.Getenv("TELEGRAM_BOT_TOKEN"))
	if err != nil {
		log.Fatal(err)
	}
	go telegram.Start(context.Background())
	server := &http.Server{
		Addr:              "0.0.0.0:" + port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
	}
	log.Fatal(server.ListenAndServe())
}

func health(w http.ResponseWriter, _ *http.Request) {
	_, err := fmt.Fprintln(w, "ok")
	if err != nil {
		return
	}
}
