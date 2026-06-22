from __future__ import annotations

import pytest

from final_review_sales.exceptions import InvalidStateTransitionError
from final_review_sales.models.conversation import ConversationState
from final_review_sales.services.state_store import StateStore
from final_review_sales.state import transition


def test_legal_state_transitions():
    state = "idle"
    for target in [
        "collecting_need",
        "package_confirmed",
        "order_created",
        "waiting_payment",
        "paid",
        "delivered",
    ]:
        state = transition(state, target)
    assert state == "delivered"


def test_unpaid_cannot_deliver():
    with pytest.raises(InvalidStateTransitionError):
        transition("waiting_payment", "delivered")


def test_expired_order_cannot_deliver():
    with pytest.raises(InvalidStateTransitionError):
        transition("expired", "delivered")


def test_restart_recovers_waiting_payment(tmp_path):
    path = tmp_path / "state.json"
    store = StateStore(path)
    store.upsert(ConversationState(
        chatPlatform="qq",
        chatUserId="123",
        state="waiting_payment",
        orderId="ord_1",
    ))
    restored = StateStore(path)
    waiting = restored.list_waiting_payment()
    assert len(waiting) == 1
    assert waiting[0].orderId == "ord_1"
