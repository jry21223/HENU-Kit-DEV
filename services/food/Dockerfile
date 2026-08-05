FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/food ./cmd/server \
  && CGO_ENABLED=0 go build -trimpath -o /out/food-healthcheck ./cmd/healthcheck

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/food /food
COPY --from=build /out/food-healthcheck /food-healthcheck
EXPOSE 8096
ENTRYPOINT ["/food"]
