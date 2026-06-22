from __future__ import annotations

from dataclasses import asdict, dataclass
from typing import Any


@dataclass
class ConversationState:
    chatPlatform: str
    chatUserId: str
    chatGroupId: str | None = None
    state: str = "idle"
    packageId: str | None = None
    orderId: str | None = None
    deliveryId: str | None = None
    expiresAt: str | None = None

    def key(self) -> str:
        group = self.chatGroupId or "private"
        return f"{self.chatPlatform}:{group}:{self.chatUserId}"

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> "ConversationState":
        return cls(
            chatPlatform=str(data["chatPlatform"]),
            chatUserId=str(data["chatUserId"]),
            chatGroupId=data.get("chatGroupId"),
            state=str(data.get("state", "idle")),
            packageId=data.get("packageId"),
            orderId=data.get("orderId"),
            deliveryId=data.get("deliveryId"),
            expiresAt=data.get("expiresAt"),
        )
