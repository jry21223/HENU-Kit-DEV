FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/library ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/library /library
EXPOSE 8095
ENTRYPOINT ["/library"]
