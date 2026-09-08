FROM python:3.11.14-slim-bookworm@sha256:65a93d69fa75478d554f4ad27c85c1e69fa184956261b4301ebaf6dbb0a3543d

ARG GETWORK_REPOSITORY=https://github.com/RyaoVen/getWork
ARG GETWORK_COMMIT=2c7800d65fb22d5094d812107c63ce94734b1c2e
ARG GETWORK_ARCHIVE_SHA256=d40f7a107173da04d5d08680c56c99ef6af5a97de76cfab41502c0b1a6f0eff9

ENV DEBIAN_FRONTEND=noninteractive \
    PLAYWRIGHT_BROWSERS_PATH=/ms-playwright \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl \
    && rm -rf /var/lib/apt/lists/* \
    && curl --fail --location --silent --show-error \
      "${GETWORK_REPOSITORY}/archive/${GETWORK_COMMIT}.tar.gz" -o /tmp/getwork.tar.gz \
    && echo "${GETWORK_ARCHIVE_SHA256}  /tmp/getwork.tar.gz" | sha256sum -c - \
    && mkdir -p /opt/getwork \
    && tar -xzf /tmp/getwork.tar.gz --strip-components=1 -C /opt/getwork \
    && rm -f /tmp/getwork.tar.gz

COPY --from=ghcr.io/astral-sh/uv:0.9.7@sha256:ba4857bf2a068e9bc0e64eed8563b065908a4cd6bfb66b531a9c424c8e25e142 /uv /uvx /usr/local/bin/

WORKDIR /opt/getwork
RUN uv sync --frozen --no-dev \
    && uv run playwright install --with-deps --only-shell chromium \
    && useradd --uid 65532 --no-create-home --shell /usr/sbin/nologin getwork \
    && mkdir -p /opt/getwork/data/logs /opt/getwork/data/sessions /opt/getwork/data/output \
    && chown -R 65532:65532 /opt/getwork/data /ms-playwright

COPY --chown=65532:65532 entrypoint.py /srv/getwork-entrypoint.py

ENV PATH=/opt/getwork/.venv/bin:$PATH \
    PYTHONPATH=/opt/getwork \
    GETWORK_MCP_HOST=0.0.0.0 \
    GETWORK_MCP_PORT=8100

USER 65532:65532
EXPOSE 8100
HEALTHCHECK --interval=10s --timeout=3s --retries=12 --start-period=10s \
  CMD python -c 'import json,urllib.request; assert json.load(urllib.request.urlopen("http://127.0.0.1:8100/healthz", timeout=2))["ok"] is True'
ENTRYPOINT ["python", "/srv/getwork-entrypoint.py"]
