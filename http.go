package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"
)

func serveHTTP(ctx context.Context) error {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", health)
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
