from __future__ import annotations

from langbot_plugin.api.definition.plugin import BasePlugin

from final_review_sales.config import load_config
from final_review_sales.services.runtime import qr_service


class finalreviewsalesagent(BasePlugin):
    async def initialize(self) -> None:
        self.config = load_config()
        qr_service.cleanup_expired(max_age_seconds=self.config.order_poll_timeout_seconds)

    def __del__(self) -> None:
        try:
            qr_service.cleanup_expired()
        except Exception:
            pass
