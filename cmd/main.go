package main

import (
	"log"
	"net/http"
	"time"

	"hls-downloader/internal/handlers"
	"hls-downloader/internal/repositories"
	"hls-downloader/internal/usecases"
)

func main() {
	repo := repositories.NewJobRepository()
	uc := usecases.NewDownloaderUsecase(repo)

	addr := ":6969"
	srv := &http.Server{
		Addr:              addr,
		ReadHeaderTimeout: 10 * time.Second,
	}
	h := handlers.NewHandler(uc, srv.Shutdown)

	dir, err := uc.OutputDir()
	if err != nil {
		log.Fatalf("cannot prepare output dir: %v", err)
	}
	log.Printf("Output directory: %s", dir)

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.Dir("public")))
	mux.HandleFunc("/api/download", h.Download)
	mux.HandleFunc("/api/jobs", h.Jobs)
	mux.HandleFunc("/api/memstats", h.MemStats)
	mux.HandleFunc("/api/cancel", h.Cancel)
	mux.HandleFunc("/api/shutdown", h.Shutdown)
	srv.Handler = mux

	log.Printf("Listening on http://localhost%s", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
	log.Println("Server stopped")
}
