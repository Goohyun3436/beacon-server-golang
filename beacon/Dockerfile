# 빌드 단계
FROM golang:1.24.5-alpine AS builder

WORKDIR /build

COPY app .
RUN go mod tidy && go mod download

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o beacon main.go
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o init_bucket init_bucket.go

# 실행 단계
FROM alpine:latest

WORKDIR /app

COPY --from=builder /build/beacon .
COPY --from=builder /build/init_bucket .
COPY docker-entrypoint.sh /app/docker-entrypoint.sh

RUN chmod +x /app/docker-entrypoint.sh

RUN ls -al /app

ENTRYPOINT ["/app/docker-entrypoint.sh"]