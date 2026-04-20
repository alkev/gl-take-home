# syntax=docker/dockerfile:1.7

# ─── build stage ────────────────────────────────────────────────
FROM golang:alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath -ldflags="-s -w" \
    -o /out/vecstore ./cmd/vecstore

# Tiny statically-linked health prober so the distroless image has something
# to exec for HEALTHCHECK. It GETs /health and exits 0 iff 200.
RUN mkdir -p /src/cmd/healthcheck && \
    printf 'package main\nimport ("net/http"; "os")\nfunc main() {\n\tr, err := http.Get("http://127.0.0.1:8888/health")\n\tif err != nil || r.StatusCode != 200 { os.Exit(1) }\n}\n' > /src/cmd/healthcheck/main.go && \
    CGO_ENABLED=0 GOOS=linux go build \
    -trimpath -ldflags="-s -w" \
    -o /out/healthcheck ./cmd/healthcheck

# ─── runtime stage ──────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/vecstore    /vecstore
COPY --from=build /out/healthcheck /healthcheck
USER nonroot:nonroot
ENV PORT=8888 VECTOR_DIMENSION=100 LOG_LEVEL=info
EXPOSE 8888
HEALTHCHECK --interval=15s --timeout=3s --start-period=5s --retries=3 \
    CMD ["/healthcheck"]
ENTRYPOINT ["/vecstore"]
