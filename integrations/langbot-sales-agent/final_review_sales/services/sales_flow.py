from __future__ import annotations

from pathlib import Path

from final_review_sales.exceptions import ApiClientError, DeliveryGuardError, UnsafeToolInputError
from final_review_sales.models.conversation import ConversationState
from final_review_sales.services.final_review_api import FinalReviewApiClient
from final_review_sales.services.message_formatter import (
    format_order_payment,
    format_package_choices,
    format_package_summary,
    format_recent_orders,
    help_text,
)
from final_review_sales.services.qrcode_service import QrCodeService
from final_review_sales.services.state_store import StateStore
from final_review_sales.state import transition


class SalesFlow:
    def __init__(
        self,
        api: FinalReviewApiClient | None = None,
        qr: QrCodeService | None = None,
        store: StateStore | None = None,
    ):
        self.api = api or FinalReviewApiClient()
        self.qr = qr or QrCodeService()
        self.store = store or StateStore(".tmp/sales_state.json")
        self.pending_choices: dict[str, list[dict]] = {}

    async def help(self) -> tuple[str, Path | None]:
        return help_text(), None

    async def search(self, query: str, identity: dict[str, str | None]) -> tuple[str, Path | None]:
        result = await self.api.search_catalog(query=query or "离散数学")
        packages = result["packages"]
        key = self._identity_key(identity)
        self.pending_choices[key] = packages
        if not packages:
            return "暂未找到匹配课程包。可以换一个课程名试试。", None
        if len(packages) == 1:
            return format_package_summary(packages[0]), None
        return format_package_choices(packages, "!购买"), None

    async def purchase(self, query: str, identity: dict[str, str | None]) -> tuple[str, Path | None]:
        key = self._identity_key(identity)
        package = await self._resolve_package(query, key)
        if package is None:
            choices = self.pending_choices.get(key, [])
            if choices:
                return format_package_choices(choices, "!购买"), None
            return "没有找到可购买的课程包。请先发送：!资料 课程名", None

        state = ConversationState(
            chatPlatform=identity["chatPlatform"] or "unknown",
            chatUserId=identity["chatUserId"] or "unknown",
            chatGroupId=identity.get("chatGroupId"),
            state="idle",
            packageId=package["id"],
        )
        state.state = transition(state.state, "collecting_need")
        state.state = transition(state.state, "package_confirmed")

        order = await self.api.create_order({
            "packageId": package["id"],
            "channel": "langbot",
            "chatPlatform": state.chatPlatform,
            "chatUserId": state.chatUserId,
            "chatGroupId": state.chatGroupId,
            "buyerRemark": "用户通过 LangBot 购买课程资料",
        })
        state.orderId = order["orderId"]
        state.expiresAt = order["expiresAt"]
        state.state = transition(state.state, "order_created")
        state.state = transition(state.state, "waiting_payment")
        self.store.upsert(state)
        qr_path = self.qr.generate(order["codeUrl"], order["orderId"])
        return format_order_payment(order), qr_path

    async def recent_orders(self, identity: dict[str, str | None]) -> tuple[str, Path | None]:
        orders = await self.api.list_recent_orders(identity["chatUserId"] or "unknown")
        return format_recent_orders(orders), None

    async def resend(self, order_id: str, identity: dict[str, str | None]) -> tuple[str, Path | None]:
        if not order_id:
            return "请提供订单号，例如：!重发 ord_xxx", None
        try:
            delivery = await self.api.resend_delivery({
                "orderId": order_id,
                "chatPlatform": identity["chatPlatform"] or "unknown",
                "chatUserId": identity["chatUserId"] or "unknown",
            })
        except DeliveryGuardError:
            return "订单尚未支付成功，不能重发下载入口。请先发送 !订单 查看状态。", None
        return f"下载入口：{delivery['downloadUrl']}\n有效期至：{delivery['expiresAt']}", None

    async def handle_command_alias(
        self,
        command: str,
        args: list[str],
        identity: dict[str, str | None],
    ) -> tuple[str, Path | None]:
        try:
            if command in {"!帮助", "帮助", "help"}:
                return await self.help()
            if command in {"!资料", "资料", "search"}:
                return await self.search(" ".join(args), identity)
            if command in {"!购买", "购买", "buy"}:
                return await self.purchase(" ".join(args), identity)
            if command in {"!订单", "订单", "orders"}:
                return await self.recent_orders(identity)
            if command in {"!重发", "重发", "resend"}:
                return await self.resend(args[0] if args else "", identity)
            return "未知命令，请发送 !帮助 查看用法。", None
        except UnsafeToolInputError:
            return "订单参数不安全，已拒绝。购买金额必须以后端课程包价格为准。", None
        except ApiClientError:
            return "课程资料服务暂时不可用，请稍后再试或联系人工处理。", None

    async def _resolve_package(self, query: str, key: str) -> dict | None:
        choices = self.pending_choices.get(key, [])
        if query.strip().isdigit() and choices:
            index = int(query.strip()) - 1
            return choices[index] if 0 <= index < len(choices) else None

        result = await self.api.search_catalog(query=query or "离散数学")
        packages = result["packages"]
        if len(packages) == 1:
            return packages[0]
        self.pending_choices[key] = packages
        return None

    @staticmethod
    def _identity_key(identity: dict[str, str | None]) -> str:
        return f"{identity.get('chatPlatform')}:{identity.get('chatGroupId') or 'private'}:{identity.get('chatUserId')}"
