FROM golang:1.26 AS builder

ENV GOPROXY=https://goproxy.cn,direct

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -p 2 -trimpath -ldflags="-s -w" -o manager .

FROM alpine:latest

ENV TZ=Asia/Shanghai

RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 appuser

WORKDIR /app

COPY --from=builder /app/manager /app/

USER appuser

EXPOSE 8081 8082

ENTRYPOINT ["/app/manager"]
