from __future__ import annotations

from final_review_sales.services.final_review_api import FinalReviewApiClient
from final_review_sales.services.qrcode_service import QrCodeService
from final_review_sales.services.sales_flow import SalesFlow
from final_review_sales.services.state_store import StateStore

api_client = FinalReviewApiClient()
state_store = StateStore(".tmp/sales_state.json")
qr_service = QrCodeService()
sales_flow = SalesFlow(api=api_client, qr=qr_service, store=state_store)
