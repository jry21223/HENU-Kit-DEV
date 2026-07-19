FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/food ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/food /food
EXPOSE 8096
ENTRYPOINT ["/food"]
