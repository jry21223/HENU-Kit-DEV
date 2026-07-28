FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/account-portfolio ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/account-portfolio /account-portfolio
EXPOSE 8097
ENTRYPOINT ["/account-portfolio"]
