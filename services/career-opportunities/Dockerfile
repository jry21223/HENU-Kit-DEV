FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/career ./cmd/server \
  && CGO_ENABLED=0 go build -trimpath -o /out/career-healthcheck ./cmd/healthcheck

FROM debian:bookworm-slim
RUN apt-get update \
  && apt-get install -y --no-install-recommends ca-certificates poppler-utils \
  && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/career /career
COPY --from=build /out/career-healthcheck /career-healthcheck
USER 65532:65532
EXPOSE 8097
ENTRYPOINT ["/career"]
