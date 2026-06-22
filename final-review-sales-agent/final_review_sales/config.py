from __future__ import annotations

import os
from dataclasses import dataclass


@dataclass(frozen=True)
class PluginConfig:
    api_base_url: str = "http://localhost:3000"
    agent_secret: str = "change-me"
    agent_id: str = "langbot-final-review-sales"
    order_poll_interval_seconds: int = 5
    order_poll_timeout_seconds: int = 900
    enable_group_sales: bool = True
    enable_private_delivery: bool = True
    enable_auto_agent_reply: bool = False
    default_school: str = "河南大学"
    default_college: str = "软件学院"
    log_level: str = "INFO"


def _bool(value: str | None, default: bool) -> bool:
    if value is None:
        return default
    return value.strip().lower() in {"1", "true", "yes", "on"}


def _int(value: str | None, default: int) -> int:
    if not value:
        return default
    try:
        parsed = int(value)
    except ValueError:
        return default
    return parsed if parsed > 0 else default


def load_config() -> PluginConfig:
    return PluginConfig(
        api_base_url=os.getenv("FINAL_REVIEW_API_BASE_URL", "http://localhost:3000"),
        agent_secret=os.getenv("FINAL_REVIEW_AGENT_SECRET", "change-me"),
        agent_id=os.getenv("FINAL_REVIEW_AGENT_ID", "langbot-final-review-sales"),
        order_poll_interval_seconds=_int(os.getenv("ORDER_POLL_INTERVAL_SECONDS"), 5),
        order_poll_timeout_seconds=_int(os.getenv("ORDER_POLL_TIMEOUT_SECONDS"), 900),
        enable_group_sales=_bool(os.getenv("ENABLE_GROUP_SALES"), True),
        enable_private_delivery=_bool(os.getenv("ENABLE_PRIVATE_DELIVERY"), True),
        enable_auto_agent_reply=_bool(os.getenv("ENABLE_AUTO_AGENT_REPLY"), False),
        default_school=os.getenv("DEFAULT_SCHOOL", "河南大学"),
        default_college=os.getenv("DEFAULT_COLLEGE", "软件学院"),
        log_level=os.getenv("LOG_LEVEL", "INFO"),
    )
