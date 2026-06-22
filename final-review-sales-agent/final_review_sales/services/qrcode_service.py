from __future__ import annotations

import re
import time
from pathlib import Path

import qrcode


class QrCodeService:
    def __init__(self, temp_dir: str | Path = ".tmp/qrcodes"):
        self.temp_dir = Path(temp_dir)
        self.temp_dir.mkdir(parents=True, exist_ok=True)

    @staticmethod
    def safe_order_id(order_id: str) -> str:
        safe = re.sub(r"[^A-Za-z0-9_-]", "_", order_id)
        return safe or "order"

    def generate(self, code_url: str, order_id: str) -> Path:
        filename = f"{self.safe_order_id(order_id)}.png"
        path = (self.temp_dir / filename).resolve()
        root = self.temp_dir.resolve()
        if root not in path.parents and path != root:
            raise ValueError("Invalid QR code output path")

        image = qrcode.make(code_url)
        image.save(path)
        return path

    def cleanup_expired(self, max_age_seconds: int = 900) -> int:
        now = time.time()
        deleted = 0
        for path in self.temp_dir.glob("*.png"):
            if now - path.stat().st_mtime > max_age_seconds:
                path.unlink(missing_ok=True)
                deleted += 1
        return deleted
