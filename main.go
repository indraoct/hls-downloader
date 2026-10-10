package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"sync"
	"time"
)

// Job represents a single HLS-to-MP4 download task.
type Job struct {
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	Filename  string    `json:"filename"`
	Status    string    `json:"status"` // queued, running, done, error, cancelled
	Error     string    `json:"error,omitempty"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at,omitempty"`

	cancel context.CancelFunc `json:"-"`
}

// maxFinishedJobs caps how many jobs are retained in memory before the
// oldest finished ones are evicted.
const maxFinishedJobs = 200

// maxConcurrentDownloads bounds how many ffmpeg processes run in parallel,
// so CPU and system RAM aren't saturated by too many workers.
const maxConcurrentDownloads = 3

// sem enforces maxConcurrentDownloads.
var sem = make(chan struct{}, maxConcurrentDownloads)

var (
	jobsMu sync.Mutex
	jobs   = map[string]*Job{}

	jobCounter int

	sanitizeRe = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)
	collapseRe = regexp.MustCompile(`_+`)
	trimRe     = regexp.MustCompile(`^_+|_+$`)
)

// tailWriter is an io.Writer that retains only the last `limit` bytes,
// so ffmpeg's verbose stderr cannot grow without bound.
type tailWriter struct {
	limit int
	buf   []byte
}

func newTailWriter(limit int) *tailWriter { return &tailWriter{limit: limit} }

func (t *tailWriter) Write(p []byte) (int, error) {
	t.buf = append(t.buf, p...)
	if len(t.buf) > 2*t.limit {
		kept := make([]byte, t.limit)
		copy(kept, t.buf[len(t.buf)-t.limit:])
		t.buf = kept
	}
	return len(p), nil
}

func (t *tailWriter) String() string {
	if len(t.buf) > t.limit {
		return string(t.buf[len(t.buf)-t.limit:])
	}
	return string(t.buf)
}

// evictJobsLocked deletes the oldest finished jobs when the job map exceeds
// maxFinishedJobs. Caller must hold jobsMu. Running jobs are never evicted.
func evictJobsLocked() {
	if len(jobs) <= maxFinishedJobs {
		return
	}
	finished := make([]*Job, 0, len(jobs))
	for _, j := range jobs {
		if j.Status == "done" || j.Status == "error" || j.Status == "cancelled" {
			finished = append(finished, j)
		}
	}
	sort.Slice(finished, func(a, b int) bool {
		return finished[a].EndedAt.Before(finished[b].EndedAt)
	})
	for len(jobs) > maxFinishedJobs && len(finished) > 0 {
		delete(jobs, finished[0].ID)
		finished = finished[1:]
	}
}

// sanitizeFilename replaces spaces and special characters with underscores
// and appends the "_ffmpeg.mp4" suffix.
func sanitizeFilename(name string) string {
	name = sanitizeRe.ReplaceAllString(name, "_")
	// Collapse consecutive underscores and trim leading/trailing underscores
	name = collapseRe.ReplaceAllString(name, "_")
	name = trimRe.ReplaceAllString(name, "")
	if name == "" {
		name = "video"
	}
	return name + "_ffmpeg.mp4"
}

// outputDir returns ~/Documents/test as an absolute path.
func outputDir() (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(u.HomeDir, "Documents", "test")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// runJob executes ffmpeg for a single job (remux HLS -> mp4).
// It blocks on the semaphore until a download slot frees, and exits early
// if the job is cancelled while queued. exec.CommandContext ensures the
// ffmpeg process is killed on cancellation — no orphaned processes.
func runJob(ctx context.Context, j *Job) {
	// Wait for a free download slot (or cancellation while queued).
	select {
	case sem <- struct{}{}:
		defer func() { <-sem }()
	case <-ctx.Done():
		finishJob(j, "cancelled", "cancelled while queued")
		return
	}

	dir, err := outputDir()
	if err != nil {
		finishJob(j, "error", err.Error())
		return
	}
	outPath := filepath.Join(dir, j.Filename)

	jobsMu.Lock()
	j.Status = "running"
	jobsMu.Unlock()

	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-y",
		"-i", j.URL,
		"-c", "copy",
		"-bsf:a", "aac_adtstoasc",
		outPath,
	)
	stderr := newTailWriter(2000)
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.Canceled {
			finishJob(j, "cancelled", "cancelled by user")
			return
		}
		finishJob(j, "error", fmt.Sprintf("%v: %s", err, stderr.String()))
		return
	}
	finishJob(j, "done", "")
}

func finishJob(j *Job, status, errMsg string) {
	jobsMu.Lock()
	defer jobsMu.Unlock()
	j.Status = status
	j.Error = errMsg
	j.EndedAt = time.Now()
}

type downloadRequest struct {
	Jobs []struct {
		URL      string `json:"url"`
		Filename string `json:"filename"`
	} `json:"jobs"`
}

func handleDownload(w http.ResponseWriter, r *http.Request) {
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

	created := []*Job{}
	for _, in := range req.Jobs {
		if in.URL == "" {
			continue
		}
		ctx, cancel := context.WithCancel(context.Background())

		jobsMu.Lock()
		jobCounter++
		j := &Job{
			ID:        fmt.Sprintf("job-%d", jobCounter),
			URL:       in.URL,
			Filename:  sanitizeFilename(in.Filename),
			Status:    "queued",
			StartedAt: time.Now(),
			cancel:    cancel,
		}
		jobs[j.ID] = j
		evictJobsLocked()
		jobsMu.Unlock()
		created = append(created, j)
		go runJob(ctx, j) // concurrent download per job, bounded by sem
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"jobs": created})
}

// handleMemStats reports Go runtime memory usage for the live chart.
func handleMemStats(w http.ResponseWriter, r *http.Request) {
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

// handleCancel cancels a queued or running job (POST /api/cancel?id=job-N).
func handleCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := r.URL.Query().Get("id")

	jobsMu.Lock()
	j, ok := jobs[id]
	if ok && (j.Status == "queued" || j.Status == "running") && j.cancel != nil {
		j.cancel() // unblocks runJob (queued) or kills ffmpeg (running)
	}
	jobsMu.Unlock()

	if !ok {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func handleJobs(w http.ResponseWriter, r *http.Request) {
	jobsMu.Lock()
	list := make([]*Job, 0, len(jobs))
	for _, j := range jobs {
		list = append(list, j)
	}
	jobsMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"jobs": list})
}

func main() {
	dir, err := outputDir()
	if err != nil {
		log.Fatalf("cannot prepare output dir: %v", err)
	}
	log.Printf("Output directory: %s", dir)

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.Dir("static")))
	mux.HandleFunc("/api/download", handleDownload)
	mux.HandleFunc("/api/jobs", handleJobs)
	mux.HandleFunc("/api/memstats", handleMemStats)
	mux.HandleFunc("/api/cancel", handleCancel)

	addr := ":6969"
	log.Printf("Listening on http://localhost%s", addr)
	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Fatal(srv.ListenAndServe())
}
