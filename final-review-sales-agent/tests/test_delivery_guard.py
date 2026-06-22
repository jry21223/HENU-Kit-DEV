from __future__ import annotations

import asyncio

import pytest

from final_review_sales.exceptions import DeliveryGuardError
from final_review_sales.services.final_review_api import FinalReviewApiClient


def run(coro):
    return asyncio.run(coro)


def test_unpaid_order_cannot_create_delivery():
    client = FinalReviewApiClient()
    order = run(client.create_order({
        "packageId": "pkg_discrete_math_2023",
        "chatPlatform": "qq",
        "chatUserId": "123",
    }))
    with pytest.raises(DeliveryGuardError):
        run(client.create_delivery({"orderId": order["orderId"], "chatUserId": "123"}))


def test_repeated_paid_delivery_returns_same_link():
    client = FinalReviewApiClient()
    order = run(client.create_order({
        "packageId": "pkg_discrete_math_2023",
        "chatPlatform": "qq",
        "chatUserId": "123",
    }))
    client.mark_order_paid(order["orderId"])
    first = run(client.create_delivery({"orderId": order["orderId"], "chatUserId": "123"}))
    second = run(client.resend_delivery({"orderId": order["orderId"], "chatUserId": "123"}))
    assert first["deliveryId"] == second["deliveryId"]
    assert first["downloadUrl"] == second["downloadUrl"]


def test_other_chat_user_cannot_resend_delivery():
    client = FinalReviewApiClient()
    order = run(client.create_order({
        "packageId": "pkg_discrete_math_2023",
        "chatPlatform": "qq",
        "chatUserId": "123",
    }))
    client.mark_order_paid(order["orderId"])
    with pytest.raises(DeliveryGuardError):
        run(client.resend_delivery({"orderId": order["orderId"], "chatUserId": "456"}))
