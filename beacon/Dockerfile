FROM golang:1.24.5-alpine AS builder

WORKDIR /build

RUN apk add --no-cache git
COPY app/go.mod app/go.sum ./
RUN go mod tidy && go mod download

COPY app .

# 실행파일 빌드
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o beacon main.go && chmod +x beacon

# ────────────────────────────────────────

FROM alpine:latest

WORKDIR /app

COPY --from=builder /build/beacon .

CMD ["./beacon"]