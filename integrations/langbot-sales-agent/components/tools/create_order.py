from __future__ import annotations

import json
from typing import Any

from langbot_plugin.api.definition.components.tool.tool import Tool
from langbot_plugin.api.entities.builtin.provider import session as provider_session

from final_review_sales.exceptions import ApiClientError, UnsafeToolInputError
from final_review_sales.services.runtime import api_client


class CreateOrder(Tool):
    async def call(self, params: dict[str, Any], session: provider_session.Session, query_id: int) -> str:
        payload = {
            "packageId": params.get("packageId"),
            "channel": "langbot",
            "chatPlatform": params.get("chatPlatform", "langbot"),
            "chatUserId": params.get("chatUserId") or str(session.sender_id or session.launcher_id),
            "chatGroupId": params.get("chatGroupId"),
            "buyerRemark": "LangBot tool create_order",
        }
        for forbidden in ["price", "amount", "paid", "entitlement", "downloadUrl"]:
            if forbidden in params:
                payload[forbidden] = params[forbidden]
        try:
            result = await api_client.create_order(payload)
        except UnsafeToolInputError as exc:
            return json.dumps({"error": str(exc), "rejected": True}, ensure_ascii=False)
        except ApiClientError as exc:
            return json.dumps({"error": str(exc), "statusCode": exc.status_code}, ensure_ascii=False)
        return json.dumps(result, ensure_ascii=False)
