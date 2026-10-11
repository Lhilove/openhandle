# Production image for the OpenHandle API.
#
# Build from the repository root so the Go module and sources are available:
#   docker build -f deploy/Dockerfile -t openhandle-api .

# ---- build stage ----
FROM golang:1.25-alpine AS build
WORKDIR /src

# Dependencies first: this layer is cached until go.mod/go.sum change.
COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend/ ./

# Static binary: no libc, no shell needed at runtime.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags "-s -w" \
      -o /out/openhandle-api ./cmd/api

# ---- runtime stage ----
FROM alpine:3.20

# ca-certificates: the API calls Solana RPC and OAuth providers over HTTPS.
# tzdata: predictable time handling on any host.
RUN apk add --no-cache ca-certificates tzdata \
 && adduser -D -H -u 10001 -s /sbin/nologin app

COPY --from=build /out/openhandle-api /usr/local/bin/openhandle-api

# Never run as root; the process only needs to read its environment.
USER app

EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/openhandle-api"]
