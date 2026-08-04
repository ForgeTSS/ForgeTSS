FROM golang:1.25-bookworm AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o forgetss ./cmd/forgetss

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=builder /app/forgetss .

EXPOSE 8080

USER nonroot:nonroot

RUN adduser --disabled-password --gecos "" nonroot 2>/dev/null || true

ENTRYPOINT ["/app/forgetss"]
