FROM docker.m.daocloud.io/library/golang:1.26.6-alpine AS build
RUN apk add --no-cache git ca-certificates
ENV GOPROXY=https://goproxy.cn,direct
WORKDIR /src
COPY go.mod go.sum ./
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 /usr/local/go/bin/go build -trimpath -ldflags="-s -w" -o /out/platform-core ./cmd/server && \
    CGO_ENABLED=0 /usr/local/go/bin/go build -trimpath -ldflags="-s -w" -o /out/grant-account-operator-role ./cmd/grant-account-operator-role

FROM docker.m.daocloud.io/library/alpine:3.22
RUN addgroup -S platform && adduser -S -G platform platform
COPY --from=build /out/platform-core /usr/local/bin/platform-core
COPY --from=build /out/grant-account-operator-role /usr/local/bin/grant-account-operator-role
USER platform
EXPOSE 8081
ENTRYPOINT ["platform-core"]
