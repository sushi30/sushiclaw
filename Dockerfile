# ============================================================
# Stage 1: Build the sushiclaw binary
# ============================================================
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git

# Clone the picoclaw sushi30 fork so the go.mod replace directive
# (replace github.com/sipeed/picoclaw => ../picoclaw) resolves at /picoclaw.
RUN git clone --depth=1 --branch sushi30 https://github.com/sushi30/picoclaw.git /picoclaw

WORKDIR /src

# Cache dependencies (picoclaw must already exist for the replace directive)
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY . .
RUN CGO_ENABLED=0 go build -mod=mod -o sushiclaw .

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
