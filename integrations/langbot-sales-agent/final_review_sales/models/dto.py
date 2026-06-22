from __future__ import annotations

from dataclasses import asdict, dataclass, field
from datetime import datetime, timedelta, timezone
from typing import Any


def utc_now() -> datetime:
    return datetime.now(timezone.utc)


def iso(dt: datetime) -> str:
    return dt.astimezone(timezone.utc).isoformat().replace("+00:00", "Z")


@dataclass
class PackageSummary:
    id: str
    title: str
    description: str
    priceFen: int
    currency: str
    school: str
    college: str
    major: str
    grade: str
    includes: list[str]
    status: str = "published"

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


@dataclass
class PackageDetail(PackageSummary):
    materials: list[dict[str, str]] = field(default_factory=list)
    accessRule: str = "paid 资料必须支付成功后通过后端 delivery 入口访问"


@dataclass
class OrderResult:
    orderId: str
    outTradeNo: str
    title: str
    amountFen: int
    currency: str
    codeUrl: str
    expiresAt: str
    status: str = "paying"

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


@dataclass
class OrderStatusResult:
    orderId: str
    status: str
    paidAt: str | None = None
    deliveryCreated: bool = False
    deliveryId: str | None = None

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


@dataclass
class DeliveryResult:
    deliveryId: str
    downloadUrl: str
    expiresAt: str

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


def default_delivery_expires_at() -> str:
    return iso(utc_now() + timedelta(days=7))
