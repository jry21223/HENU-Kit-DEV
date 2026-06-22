class FinalReviewSalesError(Exception):
    """Base exception for the sales plugin."""


class ApiClientError(FinalReviewSalesError):
    def __init__(self, message: str, status_code: int | None = None):
        super().__init__(message)
        self.status_code = status_code


class UnsafeToolInputError(FinalReviewSalesError):
    """Raised when a tool input attempts to control price, paid state, or delivery."""


class InvalidStateTransitionError(FinalReviewSalesError):
    """Raised when an order state transition is not allowed."""


class DeliveryGuardError(FinalReviewSalesError):
    """Raised when delivery is attempted before payment is confirmed."""
