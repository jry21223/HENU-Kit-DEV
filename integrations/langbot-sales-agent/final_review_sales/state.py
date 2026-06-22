from __future__ import annotations

from final_review_sales.exceptions import InvalidStateTransitionError

VALID_STATES = {
    "idle",
    "collecting_need",
    "package_confirmed",
    "order_created",
    "waiting_payment",
    "paid",
    "delivered",
    "after_sales",
    "expired",
    "failed",
}

VALID_TRANSITIONS = {
    "idle": {"collecting_need", "after_sales", "failed"},
    "collecting_need": {"package_confirmed", "after_sales", "failed"},
    "package_confirmed": {"order_created", "after_sales", "failed"},
    "order_created": {"waiting_payment", "after_sales", "failed"},
    "waiting_payment": {"paid", "expired", "after_sales", "failed"},
    "paid": {"delivered", "after_sales", "failed"},
    "delivered": {"after_sales"},
    "expired": {"after_sales", "collecting_need"},
    "failed": {"after_sales", "collecting_need"},
    "after_sales": {"collecting_need", "failed"},
}


def can_transition(current: str, target: str) -> bool:
    return current in VALID_STATES and target in VALID_TRANSITIONS.get(current, set())


def transition(current: str, target: str) -> str:
    if not can_transition(current, target):
        raise InvalidStateTransitionError(f"Invalid state transition: {current} -> {target}")
    return target
