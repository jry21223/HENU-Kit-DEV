# Food Post MCP server (ADR-0033): exposes the ADR-0032 Food Post capability
# to remote AI agents over Streamable HTTP. Fails closed without the access
# token; Food remains the sole data and policy owner.
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/food-mcp ./cmd/server

FROM alpine:3.20
RUN adduser -D -u 10001 henukit
USER henukit
COPY --from=build /out/food-mcp /food-mcp
EXPOSE 8098
ENTRYPOINT ["/food-mcp"]
