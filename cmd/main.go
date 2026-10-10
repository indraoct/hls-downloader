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
	h := handlers.NewHandler(uc)

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

	addr := ":6969"
	log.Printf("Listening on http://localhost%s", addr)
	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Fatal(srv.ListenAndServe())
}
