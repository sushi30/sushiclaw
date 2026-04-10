# ============================================================
# Stage 1: Build the sushiclaw binary
# ============================================================
FROM golang:1.25-alpine AS builder

WORKDIR /src

# Cache go module dependencies
COPY go.mod go.sum ./
# Copy picoclaw submodule — resolves the go.mod replace directive (./picoclaw)
COPY picoclaw/ picoclaw/
RUN go mod download

# Copy source and build
COPY . .
RUN CGO_ENABLED=0 go build -o sushiclaw .

# ============================================================
# Stage 2: Minimal runtime image
# ============================================================
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata

# Health check (picoclaw health server runs on 18790)
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
  CMD wget -q --spider http://localhost:18790/health || exit 1

COPY --from=builder /src/sushiclaw /usr/local/bin/sushiclaw

RUN addgroup -g 1000 sushiclaw && \
    adduser -D -u 1000 -G sushiclaw sushiclaw

USER sushiclaw

ENTRYPOINT ["sushiclaw"]
CMD ["gateway"]
