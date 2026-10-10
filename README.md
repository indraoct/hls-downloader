# HLS Downloader

A small web application to download HLS (`.m3u8`) video streams and save them as MP4 files using `ffmpeg`.

## Features

- Paste one or more m3u8 URLs and download them concurrently (max 3 ffmpeg workers at once; the rest stay queued)
- Cancel queued or running jobs — ffmpeg is killed immediately, no zombie processes
- Dynamic input rows — add/remove as many URL + filename pairs as needed
- Automatic filename sanitization: spaces and special characters become `_`, then a `_ffmpeg.mp4` suffix is appended
- Live job status (queued / running / done / error) with error details from ffmpeg
- Output saved to `~/Documents/test`
- Memory-safe: bounded job history and bounded ffmpeg log capture

## Prerequisites

- **Go** 1.26+ (any recent version works)
- **ffmpeg** installed and on `PATH`

```bash
# macOS
brew install ffmpeg
```

## Run

```bash
go build -o hls-downloader .
./hls-downloader
```

Then open **http://localhost:6969** in your browser.

## Usage

1. Enter an **m3u8 URL** (required) and a **file name** (optional — defaults to `video`).
2. Click **+ Add another** to queue multiple downloads; all run in parallel.
3. Click **Start Download**. The Jobs panel shows live progress status.
4. Finished files appear in `~/Documents/test` as `<name>_ffmpeg.mp4`.

Example: `My Movie (2024)!` → `My_Movie_2024_ffmpeg.mp4`

## API

| Method | Endpoint        | Body / Response                                        |
|--------|-----------------|--------------------------------------------------------|
| GET    | `/`             | Web UI (static files)                                  |
| POST   | `/api/download` | `{"jobs":[{"url":"...m3u8","filename":"name"}, ...]}`  |
| GET    | `/api/jobs`     | `{"jobs":[{id,url,filename,status,error,started_at}]}` |
| POST   | `/api/cancel`   | Query: `?id=job-N` — cancels a queued/running job    |
| GET    | `/api/memstats` | `{heap_alloc,heap_sys,sys,num_gc,goroutines}`        |

Example:

```bash
curl -X POST http://localhost:6969/api/download \
  -H "Content-Type: application/json" \
  -d '{"jobs":[{"url":"https://example.com/stream.m3u8","filename":"my video"}]}'
```

## How it works

Each job runs the equivalent of:

```bash
ffmpeg -y -i "<m3u8-url>" -c copy -bsf:a aac_adtstoasc "<name>_ffmpeg.mp4"
```

`-c copy` remuxes without re-encoding (fast, lossless); `aac_adtstoasc` converts the audio bitstream for the MP4 container.

## Configuration

| What                | Where                              |
|---------------------|------------------------------------|
| Port                | `main.go` — `addr := ":6969"`           |
| Output directory    | `main.go` — `outputDir()`               |
| Retained job history| `main.go` — `maxFinishedJobs`           |
| Parallel downloads  | `main.go` — `maxConcurrentDownloads`    |

## Project structure

```
hls-downloader/
├── main.go          # HTTP server, job management, ffmpeg runner
├── go.mod
├── static/
│   ├── index.html   # UI markup
│   ├── style.css    # UI styles
│   └── app.js       # Dynamic inputs + status polling
└── README.md
```
