FROM mirror.gcr.io/library/golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 /usr/local/go/bin/go build -trimpath -ldflags="-s -w" -o /out/platform-core ./cmd/server

FROM mirror.gcr.io/library/alpine:3.22
RUN addgroup -S platform && adduser -S -G platform platform
COPY --from=build /out/platform-core /usr/local/bin/platform-core
USER platform
EXPOSE 8081
ENTRYPOINT ["platform-core"]
