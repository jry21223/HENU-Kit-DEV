# Career Resume MCP server: exposes the Career resume AI-extraction capability
# to remote AI agents over Streamable HTTP. Fails closed without the access
# token; Career remains the sole data and policy owner.
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/career-mcp ./cmd/server

FROM alpine:3.20
RUN adduser -D -u 10001 henukit
USER henukit
COPY --from=build /out/career-mcp /career-mcp
EXPOSE 8099
ENTRYPOINT ["/career-mcp"]
