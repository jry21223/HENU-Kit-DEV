FROM golang:1.26.5-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/quizcraft ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/quizcraft /quizcraft
EXPOSE 8080
ENTRYPOINT ["/quizcraft"]
