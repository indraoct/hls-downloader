// Package handlers contains the HTTP transport layer.
package handlers

import (
	"encoding/json"
	"net/http"
	"runtime"
	"time"

	"hls-downloader/internal/entities"
	"hls-downloader/internal/usecases"
)

// Handler exposes the downloader usecase over HTTP.
type Handler struct {
	uc *usecases.DownloaderUsecase
}

func NewHandler(uc *usecases.DownloaderUsecase) *Handler {
	return &Handler{uc: uc}
}

type downloadRequest struct {
	Jobs []struct {
		URL      string `json:"url"`
		Filename string `json:"filename"`
	} `json:"jobs"`
}

// Download handles POST /api/download.
func (h *Handler) Download(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req downloadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if len(req.Jobs) == 0 {
		http.Error(w, "no jobs provided", http.StatusBadRequest)
		return
	}

	created := []*entities.Job{}
	for _, in := range req.Jobs {
		if in.URL == "" {
			continue
		}
		created = append(created, h.uc.Enqueue(in.URL, in.Filename))
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"jobs": created})
}

// Jobs handles GET /api/jobs.
func (h *Handler) Jobs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"jobs": h.uc.List()})
}

// Cancel handles POST /api/cancel?id=job-N and cancels a queued or
// running job.
func (h *Handler) Cancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.uc.Cancel(r.URL.Query().Get("id")) {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// MemStats handles GET /api/memstats — it reports Go runtime memory
// usage for the live chart.
func (h *Handler) MemStats(w http.ResponseWriter, r *http.Request) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"heap_alloc": m.HeapAlloc, // bytes currently allocated on heap
		"heap_sys":   m.HeapSys,   // bytes reserved from OS for heap
		"sys":        m.Sys,       // total bytes reserved from OS
		"num_gc":     m.NumGC,     // completed GC cycles
		"goroutines": runtime.NumGoroutine(),
		"timestamp":  time.Now().UnixMilli(),
	})
}
