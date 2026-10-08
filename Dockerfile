FROM golang:1.26 AS builder

ENV GOPROXY=https://goproxy.cn,direct

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o server ./cmd/api
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o worker ./cmd/worker

FROM alpine:latest

ENV TZ=Asia/Shanghai

RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 appuser

WORKDIR /app

COPY --from=builder /app/server /app/worker /app/


USER appuser

EXPOSE 8080

CMD ["/app/server"]
