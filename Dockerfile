# --- build stage ---
FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/api ./cmd/api

# --- runtime stage ---
FROM alpine:3.20

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY --from=builder /bin/api .
COPY migrations/ ./migrations/
COPY docs/api/ ./docs/api/

EXPOSE 8080

ENTRYPOINT ["./api"]
