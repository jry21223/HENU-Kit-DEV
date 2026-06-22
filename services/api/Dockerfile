FROM golang:1.23-alpine AS builder
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN go build -o /bin/final-review-api ./cmd/server

FROM alpine:3.21
WORKDIR /app
COPY --from=builder /bin/final-review-api /usr/local/bin/final-review-api
EXPOSE 8080
CMD ["final-review-api"]
