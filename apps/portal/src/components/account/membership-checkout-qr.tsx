"use client";

import { useEffect, useState } from "react";
import QRCode from "qrcode";

import Img from "@/components/ui/img";

interface MembershipCheckoutQRProps {
  checkoutURL: string;
}

/**
 * An encode result is tagged with the URI it belongs to, so a result for a
 * previous code is never shown against a newer one and "still rendering" stays
 * a derived state rather than something the effect has to write.
 */
type EncodeResult =
  | { url: string; kind: "ready"; dataURL: string }
  | { url: string; kind: "failed" };

/**
 * A WeChat payment URI is a single-use code carrying no merchant order number,
 * so it is safe to display. Anything else is refused rather than rendered,
 * which keeps a server-side regression from putting a private merchant order
 * number on screen.
 */
function isBrowserSafeCheckoutURL(value: string): boolean {
  return value.startsWith("weixin://");
}

/** Renders a WeChat payment URI as a scannable QR code. */
export function MembershipCheckoutQR({ checkoutURL }: MembershipCheckoutQRProps) {
  const safe = isBrowserSafeCheckoutURL(checkoutURL);
  const [result, setResult] = useState<EncodeResult | null>(null);
  const current = result?.url === checkoutURL ? result : null;

  useEffect(() => {
    if (!safe) return;
    let active = true;
    QRCode.toDataURL(checkoutURL, { errorCorrectionLevel: "M", margin: 1, width: 320 }).then(
      (dataURL: string) => {
        if (active) setResult({ url: checkoutURL, kind: "ready", dataURL });
      },
      () => {
        if (active) setResult({ url: checkoutURL, kind: "failed" });
      }
    );
    return () => {
      active = false;
    };
  }, [checkoutURL, safe]);

  if (!safe || current?.kind === "failed") {
    return (
      <div
        data-membership-checkout-qr="error"
        role="alert"
        className="flex aspect-square w-full max-w-[280px] items-center justify-center border border-accent p-6 text-center"
      >
        <p className="text-sm leading-6 text-ink/70">
          支付二维码无法显示。请重新发起支付，不要扫描其他来源的二维码。
        </p>
      </div>
    );
  }

  if (current === null) {
    return (
      <div
        data-membership-checkout-qr="rendering"
        aria-live="polite"
        className="flex aspect-square w-full max-w-[280px] items-center justify-center border border-line font-mono text-xs tracking-[0.2em] text-ink/50"
      >
        QR RENDERING<span className="animate-pulse text-accent">…</span>
      </div>
    );
  }

  return (
    <div data-membership-checkout-qr="ready" className="w-full max-w-[280px]">
      <Img src={current.dataURL} alt="微信支付二维码" label="QR" className="w-full bg-white" />
    </div>
  );
}
