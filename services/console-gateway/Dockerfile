FROM docker.m.daocloud.io/library/golang:1.26.5-alpine AS build
RUN apk add --no-cache git ca-certificates
ENV GOPROXY=https://goproxy.cn,direct
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod /usr/local/go/bin/go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 /usr/local/go/bin/go build -trimpath -ldflags="-s -w" -o /out/console-gateway ./cmd/server

FROM docker.m.daocloud.io/library/alpine:3.22
RUN addgroup -S gateway && adduser -S -G gateway gateway
COPY --from=build /out/console-gateway /usr/local/bin/console-gateway
USER gateway
EXPOSE 8082
ENTRYPOINT ["/usr/local/bin/console-gateway"]
