FROM python:3.12-slim AS runtime
WORKDIR /app

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    APP_HOST=0.0.0.0 \
    APP_PORT=10086 \
    PIP_DISABLE_PIP_VERSION_CHECK=1 \
    PIP_DEFAULT_TIMEOUT=120

COPY requirements.docker.txt ./
# China-only index to avoid files.pythonhosted.org timeouts on this host.
RUN pip install --no-cache-dir -r requirements.docker.txt \
      -i https://pypi.tuna.tsinghua.edu.cn/simple \
      --trusted-host pypi.tuna.tsinghua.edu.cn

COPY server.py db_storage.py llm_service.py ./
COPY scripts ./scripts
COPY tiku ./tiku
COPY generated ./generated

EXPOSE 10086
CMD ["python", "server.py"]
