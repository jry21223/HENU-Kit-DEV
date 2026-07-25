FROM python:3.12-slim AS runtime
WORKDIR /app

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    APP_HOST=0.0.0.0 \
    APP_PORT=10086

COPY requirements.txt ./
RUN pip install --no-cache-dir -r requirements.txt

COPY server.py db_storage.py llm_service.py ./
COPY scripts ./scripts
COPY tiku ./tiku
COPY generated ./generated

EXPOSE 10086
CMD ["python", "server.py"]
