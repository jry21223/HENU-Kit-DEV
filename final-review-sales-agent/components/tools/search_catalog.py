from __future__ import annotations

import json
from typing import Any

from langbot_plugin.api.definition.components.tool.tool import Tool
from langbot_plugin.api.entities.builtin.provider import session as provider_session

from final_review_sales.services.runtime import api_client


class SearchCatalog(Tool):
    async def call(self, params: dict[str, Any], session: provider_session.Session, query_id: int) -> str:
        result = await api_client.search_catalog(
            query=str(params.get("query", "")),
            school=str(params.get("school", "河南大学")),
            college=str(params.get("college", "软件学院")),
            major=params.get("major"),
            grade=params.get("grade"),
        )
        return json.dumps(result, ensure_ascii=False)
