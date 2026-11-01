# syntax=docker/dockerfile:1

# ---------- build ----------
FROM golang:1.27-trixie AS build
WORKDIR /src

# dependency di-download di layer terpisah, jadi di-cache selama go.mod/go.sum tidak berubah.
# --mount=type=cache menyimpan module & hasil compile antar build, jadi build ulang jauh lebih cepat
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .
# CGO_ENABLED=0: binary statis, tidak butuh library C di image runtime
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

# ---------- runtime ----------
FROM debian:trixie-slim

# poppler-utils: pdftotext, pdftoppm, pdfimages, pdfinfo (dipakai extractor)
# ca-certificates: koneksi HTTPS ke Supabase
# tzdata: supaya waktu di log bisa diatur lewat TZ
RUN apt-get update \
    && apt-get install -y --no-install-recommends poppler-utils ca-certificates tzdata \
    && rm -rf /var/lib/apt/lists/*

# jangan jalan sebagai root
RUN useradd --system --uid 10001 app
USER app

WORKDIR /app
COPY --from=build /out/api /app/api

ENV PORT=8081
EXPOSE 8081
CMD ["/app/api"]
