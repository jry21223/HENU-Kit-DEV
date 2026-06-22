from __future__ import annotations

import json
from typing import Any

from langbot_plugin.api.definition.components.tool.tool import Tool
from langbot_plugin.api.entities.builtin.provider import session as provider_session

from final_review_sales.exceptions import ApiClientError
from final_review_sales.services.runtime import api_client


class QueryOrder(Tool):
    async def call(self, params: dict[str, Any], session: provider_session.Session, query_id: int) -> str:
        try:
            result = await api_client.query_order(str(params.get("orderId", "")))
        except ApiClientError as exc:
            return json.dumps({"error": str(exc), "statusCode": exc.status_code}, ensure_ascii=False)
        return json.dumps(result, ensure_ascii=False)
