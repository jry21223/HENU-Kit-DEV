from __future__ import annotations

import asyncio

from final_review_sales.models.conversation import ConversationState
from final_review_sales.services.final_review_api import FinalReviewApiClient
from final_review_sales.services.order_watcher import OrderWatcher
from final_review_sales.services.state_store import StateStore


def run(coro):
    return asyncio.run(coro)


def test_search_create_order_waiting_payment(tmp_path):
    client = FinalReviewApiClient()
    order = run(client.create_order({
        "packageId": "pkg_discrete_math_2023",
        "chatPlatform": "qq",
        "chatUserId": "123",
    }))
    assert order["status"] == "paying"
    status = run(client.query_order(order["orderId"]))
    assert status["status"] == "paying"
    assert status["deliveryCreated"] is False


def test_paid_order_creates_delivery_once(tmp_path):
    client = FinalReviewApiClient()
    store = StateStore(tmp_path / "state.json")
    watcher = OrderWatcher(client, store, poll_interval_seconds=0, timeout_seconds=1)
    order = run(client.create_order({
        "packageId": "pkg_discrete_math_2023",
        "chatPlatform": "qq",
        "chatUserId": "123",
    }))
    state = ConversationState(chatPlatform="qq", chatUserId="123", state="waiting_payment", orderId=order["orderId"])
    store.upsert(state)
    client.mark_order_paid(order["orderId"])
    result = run(watcher.watch_once(state))
    assert result.state == "delivered"
    first_delivery = result.delivery["deliveryId"]
    result_again = run(watcher.watch_once(state))
    assert result_again.state == "delivered"
    status = run(client.query_order(order["orderId"]))
    assert status["deliveryId"] == first_delivery


def test_expired_order_does_not_deliver(tmp_path):
    client = FinalReviewApiClient()
    store = StateStore(tmp_path / "state.json")
    watcher = OrderWatcher(client, store, poll_interval_seconds=0, timeout_seconds=1)
    order = run(client.create_order({
        "packageId": "pkg_discrete_math_2023",
        "chatPlatform": "qq",
        "chatUserId": "123",
    }))
    state = ConversationState(chatPlatform="qq", chatUserId="123", state="waiting_payment", orderId=order["orderId"])
    client.mark_order_expired(order["orderId"])
    result = run(watcher.watch_once(state))
    assert result.state == "expired"
    assert result.delivery is None
