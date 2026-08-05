FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/notice ./cmd/server \
  && CGO_ENABLED=0 go build -trimpath -o /out/notice-healthcheck ./cmd/healthcheck

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/notice /notice
COPY --from=build /out/notice-healthcheck /notice-healthcheck
EXPOSE 8094
ENTRYPOINT ["/notice"]
