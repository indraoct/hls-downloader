// Package usecases contains the application business logic.
package usecases

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"sync"

	"hls-downloader/internal/entities"
	"hls-downloader/internal/repositories"
)

// maxConcurrentDownloads bounds how many ffmpeg processes run in parallel,
// so CPU and system RAM aren't saturated by too many workers.
const maxConcurrentDownloads = 3

var (
	sanitizeRe = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)
	collapseRe = regexp.MustCompile(`_+`)
	trimRe     = regexp.MustCompile(`^_+|_+$`)
)

// DownloaderUsecase orchestrates HLS-to-MP4 download jobs via ffmpeg.
type DownloaderUsecase struct {
	repo *repositories.JobRepository
	// sem enforces maxConcurrentDownloads.
	sem chan struct{}

	mu      sync.Mutex
	cancels map[string]context.CancelFunc
}

func NewDownloaderUsecase(repo *repositories.JobRepository) *DownloaderUsecase {
	return &DownloaderUsecase{
		repo:    repo,
		sem:     make(chan struct{}, maxConcurrentDownloads),
		cancels: map[string]context.CancelFunc{},
	}
}

// Enqueue registers a new download job and starts its worker goroutine.
// Concurrent downloads are bounded by the semaphore.
func (u *DownloaderUsecase) Enqueue(url, filename string) *entities.Job {
	ctx, cancel := context.WithCancel(context.Background())
	j := u.repo.Create(url, sanitizeFilename(filename))

	u.mu.Lock()
	u.cancels[j.ID] = cancel
	u.mu.Unlock()

	go u.runJob(ctx, j)
	return j
}

// List returns all known jobs.
func (u *DownloaderUsecase) List() []*entities.Job {
	return u.repo.List()
}

// Cancel cancels a queued or running job. It returns false if the job
// is unknown.
func (u *DownloaderUsecase) Cancel(id string) bool {
	status, ok := u.repo.Status(id)
	if !ok {
		return false
	}
	if status != "queued" && status != "running" {
		return true
	}

	u.mu.Lock()
	cancel, ok := u.cancels[id]
	u.mu.Unlock()
	if ok {
		cancel() // unblocks runJob (queued) or kills ffmpeg (running)
	}
	return true
}

// OutputDir returns ~/Documents/test as an absolute path, creating it
// if needed.
func (u *DownloaderUsecase) OutputDir() (string, error) {
	usr, err := user.Current()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(usr.HomeDir, "Documents", "test")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// runJob executes ffmpeg for a single job (remux HLS -> mp4).
// It blocks on the semaphore until a download slot frees, and exits early
// if the job is cancelled while queued. exec.CommandContext ensures the
// ffmpeg process is killed on cancellation — no orphaned processes.
func (u *DownloaderUsecase) runJob(ctx context.Context, j *entities.Job) {
	defer u.clearCancel(j.ID)

	// Wait for a free download slot (or cancellation while queued).
	select {
	case u.sem <- struct{}{}:
		defer func() { <-u.sem }()
	case <-ctx.Done():
		u.finishJob(j, "cancelled", "cancelled while queued")
		return
	}

	dir, err := u.OutputDir()
	if err != nil {
		u.finishJob(j, "error", err.Error())
		return
	}
	outPath := filepath.Join(dir, j.Filename)

	u.repo.SetStatus(j.ID, "running", "")

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
			u.finishJob(j, "cancelled", "cancelled by user")
			return
		}
		u.finishJob(j, "error", fmt.Sprintf("%v: %s", err, stderr.String()))
		return
	}
	u.finishJob(j, "done", "")
}

func (u *DownloaderUsecase) finishJob(j *entities.Job, status, errMsg string) {
	u.repo.SetStatus(j.ID, status, errMsg)
}

func (u *DownloaderUsecase) clearCancel(id string) {
	u.mu.Lock()
	delete(u.cancels, id)
	u.mu.Unlock()
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
