FROM mirror.gcr.io/library/golang:1.26.5-alpine AS build
ENV GOPROXY="https://proxy.golang.org|direct"
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod /usr/local/go/bin/go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 /usr/local/go/bin/go build -trimpath -ldflags="-s -w" -o /out/console-gateway ./cmd/server

FROM mirror.gcr.io/library/alpine:3.22
RUN addgroup -S gateway && adduser -S -G gateway gateway
COPY --from=build /out/console-gateway /usr/local/bin/console-gateway
USER gateway
EXPOSE 8082
ENTRYPOINT ["/usr/local/bin/console-gateway"]
