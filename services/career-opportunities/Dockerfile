FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/career ./cmd/server \
  && CGO_ENABLED=0 go build -trimpath -o /out/career-healthcheck ./cmd/healthcheck

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/career /career
COPY --from=build /out/career-healthcheck /career-healthcheck
EXPOSE 8097
ENTRYPOINT ["/career"]
