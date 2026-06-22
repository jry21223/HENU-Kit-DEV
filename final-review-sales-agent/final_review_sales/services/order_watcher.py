from __future__ import annotations

import asyncio
from dataclasses import dataclass

from final_review_sales.exceptions import DeliveryGuardError
from final_review_sales.models.conversation import ConversationState
from final_review_sales.services.final_review_api import FinalReviewApiClient
from final_review_sales.services.state_store import StateStore
from final_review_sales.state import transition


@dataclass
class WatchResult:
    state: str
    delivery: dict | None = None


class OrderWatcher:
    def __init__(
        self,
        api: FinalReviewApiClient,
        store: StateStore,
        poll_interval_seconds: int = 5,
        timeout_seconds: int = 900,
    ):
        self.api = api
        self.store = store
        self.poll_interval_seconds = poll_interval_seconds
        self.timeout_seconds = timeout_seconds

    async def watch_once(self, state: ConversationState) -> WatchResult:
        if state.state != "waiting_payment" or not state.orderId:
            return WatchResult(state=state.state)

        status = await self.api.query_order(state.orderId)
        if status["status"] == "paid":
            state.state = transition(state.state, "paid")
            delivery = await self.api.create_delivery({
                "orderId": state.orderId,
                "chatPlatform": state.chatPlatform,
                "chatUserId": state.chatUserId,
            })
            state.state = transition(state.state, "delivered")
            state.deliveryId = delivery["deliveryId"]
            self.store.upsert(state)
            return WatchResult(state=state.state, delivery=delivery)

        if status["status"] in {"expired", "closed"}:
            state.state = transition(state.state, "expired")
            self.store.upsert(state)
            return WatchResult(state=state.state)

        if status["status"] == "failed":
            state.state = transition(state.state, "failed")
            self.store.upsert(state)
            return WatchResult(state=state.state)

        return WatchResult(state=state.state)

    async def watch_until_done(self, state: ConversationState) -> WatchResult:
        elapsed = 0
        while elapsed <= self.timeout_seconds:
            result = await self.watch_once(state)
            if result.state in {"delivered", "expired", "failed"}:
                return result
            await asyncio.sleep(self.poll_interval_seconds)
            elapsed += self.poll_interval_seconds

        state.state = transition(state.state, "expired")
        self.store.upsert(state)
        return WatchResult(state="expired")

    async def resume_waiting(self) -> list[WatchResult]:
        results: list[WatchResult] = []
        for state in self.store.list_waiting_payment():
            try:
                results.append(await self.watch_once(state))
            except DeliveryGuardError:
                state.state = transition(state.state, "failed")
                self.store.upsert(state)
                results.append(WatchResult(state="failed"))
        return results
