FROM golang:1.25-alpine AS builder
RUN apk add --no-cache git ca-certificates
ENV GOPROXY=https://goproxy.cn,direct
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN go build -o /bin/final-review-worker ./cmd/worker

FROM alpine:3.21
WORKDIR /app
COPY --from=builder /bin/final-review-worker /usr/local/bin/final-review-worker
EXPOSE 9090
CMD ["final-review-worker"]
