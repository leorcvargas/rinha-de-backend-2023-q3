FROM golang:1.21 as builder

WORKDIR /app

COPY go.* ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=1 go build -tags "fts5" -ldflags='-s -w -extldflags "-static"' -v -o ./bin/rinha ./cmd/rinha.go

FROM debian:buster-slim
RUN set -x && apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y \
  ca-certificates && \
  rm -rf /var/lib/apt/lists/*

ENV GIN_MODE release

EXPOSE 8080

COPY --from=builder /app/bin/rinha .

CMD ["./rinha"]
