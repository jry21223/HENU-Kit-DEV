FROM mirror.gcr.io/library/golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN /usr/local/go/bin/go mod download
COPY . .
RUN CGO_ENABLED=0 /usr/local/go/bin/go build -trimpath -ldflags="-s -w" -o /out/platform-core ./cmd/server

FROM mirror.gcr.io/library/alpine:3.22
RUN addgroup -S platform && adduser -S -G platform platform
COPY --from=build /out/platform-core /usr/local/bin/platform-core
USER platform
EXPOSE 8081
ENTRYPOINT ["platform-core"]
