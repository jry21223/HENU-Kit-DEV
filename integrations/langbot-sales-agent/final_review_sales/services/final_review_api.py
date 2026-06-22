from __future__ import annotations

import asyncio
import uuid
from datetime import timedelta
from typing import Any

import httpx

from final_review_sales.config import PluginConfig, load_config
from final_review_sales.exceptions import ApiClientError, DeliveryGuardError, UnsafeToolInputError
from final_review_sales.models.dto import (
    DeliveryResult,
    OrderResult,
    OrderStatusResult,
    PackageDetail,
    PackageSummary,
    default_delivery_expires_at,
    iso,
    utc_now,
)

FORBIDDEN_ORDER_FIELDS = {"price", "amount", "paid", "entitlement", "downloadUrl"}


class FinalReviewApiClient:
    """Mock-first API client for the first LangBot plugin round.

    The shape mirrors the planned final-review-platform Agent API. Real HTTP mode is
    intentionally not used by commands in this round.
    """

    def __init__(self, config: PluginConfig | None = None, timeout_seconds: float = 8.0):
        self.config = config or load_config()
        self.timeout_seconds = timeout_seconds
        self._packages = self._build_mock_packages()
        self._orders: dict[str, dict[str, Any]] = {}
        self._deliveries: dict[str, DeliveryResult] = {}
        self._user_orders: dict[str, list[str]] = {}

    @staticmethod
    def _build_mock_packages() -> list[PackageDetail]:
        return [
            PackageDetail(
                id="pkg_discrete_math_2023",
                title="离散数学期末复习包",
                description="适合河南大学软件学院 2023级 网络工程使用",
                priceFen=1990,
                currency="CNY",
                school="河南大学",
                college="软件学院",
                major="网络工程",
                grade="2023级",
                includes=["重点知识点讲义", "模拟卷一", "模拟卷二", "答案解析", "考前速背版"],
                materials=[
                    {"title": "离散数学重点知识点讲义", "type": "knowledge_note", "accessLevel": "login_required"},
                    {"title": "离散数学模拟卷一", "type": "mock_paper", "accessLevel": "paid"},
                    {"title": "离散数学答案解析", "type": "answer", "accessLevel": "paid"},
                ],
            ),
            PackageDetail(
                id="pkg_probability_2023",
                title="概率论与数理统计A期末复习包",
                description="适合河南大学软件学院 2023级 软件工程/网络工程使用",
                priceFen=1990,
                currency="CNY",
                school="河南大学",
                college="软件学院",
                major="软件工程",
                grade="2023级",
                includes=["重点知识点讲义", "模拟卷一", "答案解析"],
                materials=[
                    {"title": "概率论重点知识点讲义", "type": "knowledge_note", "accessLevel": "login_required"},
                    {"title": "概率论模拟卷一", "type": "mock_paper", "accessLevel": "paid"},
                ],
            ),
            PackageDetail(
                id="pkg_discrete_math_2024",
                title="离散数学期末复习包",
                description="适合河南大学软件学院 2024级 网络工程使用",
                priceFen=1990,
                currency="CNY",
                school="河南大学",
                college="软件学院",
                major="网络工程",
                grade="2024级",
                includes=["重点知识点讲义", "模拟卷一", "答案解析"],
                materials=[
                    {"title": "离散数学 2024 重点讲义", "type": "knowledge_note", "accessLevel": "login_required"},
                ],
            ),
        ]

    async def search_catalog(
        self,
        query: str,
        school: str = "河南大学",
        college: str = "软件学院",
        major: str | None = None,
        grade: str | None = None,
    ) -> dict[str, Any]:
        query = query.strip()
        packages = [
            package
            for package in self._packages
            if package.status == "published"
            and (not query or query in package.title or query in package.description)
            and package.school == school
            and package.college == college
            and (major is None or package.major == major)
            and (grade is None or package.grade == grade)
        ]
        return {
            "packages": [
                PackageSummary(
                    id=package.id,
                    title=package.title,
                    description=package.description,
                    priceFen=package.priceFen,
                    currency=package.currency,
                    school=package.school,
                    college=package.college,
                    major=package.major,
                    grade=package.grade,
                    includes=package.includes,
                    status=package.status,
                ).to_dict()
                for package in packages
            ]
        }

    async def get_package_detail(self, package_id: str) -> dict[str, Any]:
        package = self._find_package(package_id)
        return package.to_dict()

    async def create_order(self, payload: dict[str, Any]) -> dict[str, Any]:
        forbidden = FORBIDDEN_ORDER_FIELDS.intersection(payload.keys())
        if forbidden:
            raise UnsafeToolInputError(f"Forbidden order fields: {', '.join(sorted(forbidden))}")

        package_id = str(payload["packageId"])
        package = self._find_package(package_id)
        order_id = f"ord_{uuid.uuid4().hex[:12]}"
        expires_at = utc_now() + timedelta(minutes=15)
        order = OrderResult(
            orderId=order_id,
            outTradeNo=f"FR{utc_now().strftime('%Y%m%d%H%M%S')}{uuid.uuid4().hex[:6]}",
            title=package.title,
            amountFen=package.priceFen,
            currency=package.currency,
            codeUrl=f"weixin://wxpay/bizpayurl?pr=mock-{order_id}",
            expiresAt=iso(expires_at),
        )
        chat_user_id = str(payload.get("chatUserId", "unknown"))
        self._orders[order_id] = {
            "order": order,
            "packageId": package_id,
            "status": "paying",
            "paidAt": None,
            "deliveryId": None,
            "chatPlatform": payload.get("chatPlatform", "mock"),
            "chatUserId": chat_user_id,
            "chatGroupId": payload.get("chatGroupId"),
        }
        self._user_orders.setdefault(chat_user_id, []).insert(0, order_id)
        return order.to_dict()

    async def query_order(self, order_id: str) -> dict[str, Any]:
        record = self._get_order_record(order_id)
        return OrderStatusResult(
            orderId=order_id,
            status=record["status"],
            paidAt=record["paidAt"],
            deliveryCreated=record["deliveryId"] is not None,
            deliveryId=record["deliveryId"],
        ).to_dict()

    async def list_recent_orders(self, chat_user_id: str, limit: int = 5) -> list[dict[str, Any]]:
        order_ids = self._user_orders.get(str(chat_user_id), [])[:limit]
        orders: list[dict[str, Any]] = []
        for order_id in order_ids:
            record = self._get_order_record(order_id)
            order: OrderResult = record["order"]
            orders.append({
                "orderId": order_id,
                "title": order.title,
                "amountFen": order.amountFen,
                "status": record["status"],
                "deliveryId": record["deliveryId"],
            })
        return orders

    async def create_delivery(self, payload: dict[str, Any]) -> dict[str, Any]:
        order_id = str(payload["orderId"])
        record = self._get_order_record(order_id)
        if payload.get("chatUserId") and str(payload["chatUserId"]) != str(record["chatUserId"]):
            raise DeliveryGuardError("Delivery can only be created for the original chat user")
        if record["status"] != "paid":
            raise DeliveryGuardError("Only paid orders can create delivery links")

        if record["deliveryId"]:
            return self._deliveries[record["deliveryId"]].to_dict()

        delivery = DeliveryResult(
            deliveryId=f"del_{uuid.uuid4().hex[:12]}",
            downloadUrl=f"https://example.com/delivery/{uuid.uuid4().hex}",
            expiresAt=default_delivery_expires_at(),
        )
        record["deliveryId"] = delivery.deliveryId
        self._deliveries[delivery.deliveryId] = delivery
        return delivery.to_dict()

    async def resend_delivery(self, payload: dict[str, Any]) -> dict[str, Any]:
        return await self.create_delivery(payload)

    def mark_order_paid(self, order_id: str) -> None:
        record = self._get_order_record(order_id)
        record["status"] = "paid"
        record["paidAt"] = iso(utc_now())

    def mark_order_expired(self, order_id: str) -> None:
        record = self._get_order_record(order_id)
        record["status"] = "expired"

    def _find_package(self, package_id: str) -> PackageDetail:
        for package in self._packages:
            if package.id == package_id:
                return package
        raise ApiClientError("Package not found", status_code=404)

    def _get_order_record(self, order_id: str) -> dict[str, Any]:
        if order_id not in self._orders:
            raise ApiClientError("Order not found", status_code=404)
        return self._orders[order_id]

    async def request_json(self, method: str, path: str, json_body: dict[str, Any] | None = None) -> dict[str, Any]:
        request_id = uuid.uuid4().hex
        headers = {
            "Authorization": f"Bearer {self.config.agent_secret}",
            "X-Agent-Id": self.config.agent_id,
            "X-Request-Id": request_id,
        }
        try:
            async with httpx.AsyncClient(timeout=self.timeout_seconds) as client:
                response = await client.request(
                    method,
                    f"{self.config.api_base_url.rstrip('/')}{path}",
                    json=json_body,
                    headers=headers,
                )
        except httpx.TimeoutException as exc:
            raise ApiClientError("Final Review API timeout") from exc
        except httpx.RequestError as exc:
            raise ApiClientError("Final Review API unavailable") from exc

        if response.status_code == 401:
            raise ApiClientError("Final Review API authentication failed", status_code=401)
        if response.status_code >= 500:
            raise ApiClientError("Final Review API temporarily unavailable", status_code=response.status_code)
        if response.status_code >= 400:
            raise ApiClientError("Final Review API request failed", status_code=response.status_code)

        await asyncio.sleep(0)
        return response.json()
