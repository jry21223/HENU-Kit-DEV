from __future__ import annotations

import os
import time

from final_review_sales.services.qrcode_service import QrCodeService


def test_code_url_generates_qrcode(tmp_path):
    service = QrCodeService(tmp_path)
    path = service.generate("weixin://wxpay/bizpayurl?pr=mock", "ord_123")
    assert path.exists()
    assert path.name == "ord_123.png"


def test_filename_is_safe(tmp_path):
    service = QrCodeService(tmp_path)
    path = service.generate("weixin://wxpay/bizpayurl?pr=mock", "../secret")
    assert path.exists()
    assert path.parent == tmp_path.resolve()
    assert ".." not in path.name


def test_cleanup_expired_files(tmp_path):
    service = QrCodeService(tmp_path)
    path = service.generate("weixin://wxpay/bizpayurl?pr=mock", "ord_old")
    old_time = time.time() - 1000
    os.utime(path, (old_time, old_time))
    assert service.cleanup_expired(max_age_seconds=10) == 1
    assert not path.exists()
