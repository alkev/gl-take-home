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

# Pre-seed /data in the build stage so the runtime image declares it as
# owned by the distroless nonroot UID (65532). Docker populates empty
# named volumes from the image's directory contents on first mount,
# inheriting this ownership — without this, /data defaults to root and
# the nonroot process can't write snapshots.
RUN install -d -o 65532 -g 65532 /out/data

# ─── runtime stage ──────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/vecstore    /vecstore
COPY --from=build /out/healthcheck /healthcheck
COPY --from=build --chown=65532:65532 /out/data /data
USER nonroot:nonroot
ENV PORT=8888 VECTOR_DIMENSION=100 LOG_LEVEL=info
EXPOSE 8888
HEALTHCHECK --interval=15s --timeout=3s --start-period=5s --retries=3 \
    CMD ["/healthcheck"]
ENTRYPOINT ["/vecstore"]
