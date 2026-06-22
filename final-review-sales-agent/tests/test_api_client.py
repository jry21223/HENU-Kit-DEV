from __future__ import annotations

import asyncio

import pytest

from final_review_sales.exceptions import ApiClientError, UnsafeToolInputError
from final_review_sales.services.final_review_api import FinalReviewApiClient


def run(coro):
    return asyncio.run(coro)


def test_search_catalog_returns_mock_package():
    client = FinalReviewApiClient()
    result = run(client.search_catalog("离散数学"))
    assert result["packages"]
    assert result["packages"][0]["status"] == "published"


def test_create_order_does_not_accept_amount_fields():
    client = FinalReviewApiClient()
    with pytest.raises(UnsafeToolInputError):
        run(client.create_order({
            "packageId": "pkg_discrete_math_2023",
            "chatUserId": "123",
            "amount": 1,
        }))


def test_query_missing_order_returns_api_error():
    client = FinalReviewApiClient()
    with pytest.raises(ApiClientError) as exc:
        run(client.query_order("ord_missing"))
    assert exc.value.status_code == 404


def test_request_json_handles_unavailable_backend():
    client = FinalReviewApiClient()
    client.config = type("Cfg", (), {
        "api_base_url": "http://127.0.0.1:9",
        "agent_secret": "secret",
        "agent_id": "agent",
    })()
    with pytest.raises(ApiClientError):
        run(client.request_json("GET", "/api/agent/catalog/search"))
