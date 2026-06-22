from __future__ import annotations

import json
from typing import Any

from langbot_plugin.api.definition.components.tool.tool import Tool
from langbot_plugin.api.entities.builtin.provider import session as provider_session

from final_review_sales.exceptions import ApiClientError, DeliveryGuardError
from final_review_sales.services.runtime import api_client


class CreateDelivery(Tool):
    async def call(self, params: dict[str, Any], session: provider_session.Session, query_id: int) -> str:
        try:
            result = await api_client.create_delivery({
                "orderId": params.get("orderId"),
                "chatPlatform": params.get("chatPlatform", "langbot"),
                "chatUserId": params.get("chatUserId") or str(session.sender_id or session.launcher_id),
            })
        except DeliveryGuardError as exc:
            return json.dumps({"error": str(exc), "deliveryCreated": False}, ensure_ascii=False)
        except ApiClientError as exc:
            return json.dumps({"error": str(exc), "statusCode": exc.status_code}, ensure_ascii=False)
        return json.dumps(result, ensure_ascii=False)
