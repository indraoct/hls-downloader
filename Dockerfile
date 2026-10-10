# syntax=docker/dockerfile:1

# ---- Build stage ----
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/hls-downloader ./cmd

# ---- Runtime stage ----
FROM alpine:3.21
RUN apk add --no-cache ffmpeg ca-certificates
WORKDIR /app
COPY --from=build /out/hls-downloader ./hls-downloader
COPY public/ ./public/
ENV OUTPUT_DIR=/downloads
EXPOSE 6969
VOLUME ["/downloads"]
CMD ["./hls-downloader"]
