"""Run the authorized upstream getWork MCP over internal Streamable HTTP."""

from __future__ import annotations

import hmac
import os

import uvicorn
from starlette.requests import Request
from starlette.responses import JSONResponse, PlainTextResponse
from starlette.routing import Route
from starlette.types import ASGIApp, Receive, Scope, Send

from getwork.server import server

ALLOWED_TOOLS = frozenset({"list_sources", "crawl_jobs"})
PINNED_UPSTREAM_TOOLS = ALLOWED_TOOLS | frozenset(
    {"login", "logout", "add_source", "render_briefing", "send_email"}
)


class BearerAuthMiddleware:
    """Keep all MCP tools behind one deployment-owned bearer credential."""

    def __init__(self, app: ASGIApp, token: str) -> None:
        self.app = app
        self.token = token

    async def __call__(self, scope: Scope, receive: Receive, send: Send) -> None:
        if scope.get("type") != "http" or scope.get("path") == "/healthz":
            await self.app(scope, receive, send)
            return
        headers = {key.lower(): value for key, value in scope.get("headers", [])}
        expected = b"Bearer " + self.token.encode("utf-8")
        if not hmac.compare_digest(headers.get(b"authorization", b""), expected):
            response = PlainTextResponse("unauthorized", status_code=401)
            await response(scope, receive, send)
            return
        await self.app(scope, receive, send)


async def healthz(_request: Request) -> JSONResponse:
    return JSONResponse({"ok": True, "upstream": "RyaoVen/getWork@2c7800d"})


def required_token() -> str:
    token = os.environ.get("GETWORK_MCP_ACCESS_TOKEN", "").strip()
    lowered = token.lower()
    if len(token) < 32 or any(
        marker in lowered for marker in ("replace", "example", "change-me", "test-only")
    ):
        raise RuntimeError(
            "GETWORK_MCP_ACCESS_TOKEN must be a non-placeholder value of at least 32 characters"
        )
    return token


def restrict_tools() -> None:
    """Remove upstream capabilities that Career is not authorized to invoke."""

    for name in PINNED_UPSTREAM_TOOLS - ALLOWED_TOOLS:
        server.remove_tool(name)


def create_app(token: str) -> ASGIApp:
    restrict_tools()
    app = server.streamable_http_app(
        streamable_http_path="/mcp",
        json_response=True,
        stateless_http=True,
        host="0.0.0.0",
    )
    app.routes.append(Route("/healthz", healthz, methods=["GET"]))
    return BearerAuthMiddleware(app, token)


def main() -> None:
    host = os.environ.get("GETWORK_MCP_HOST", "0.0.0.0")
    port = int(os.environ.get("GETWORK_MCP_PORT", "8100"))
    uvicorn.run(create_app(required_token()), host=host, port=port, access_log=False)


if __name__ == "__main__":
    main()
