import { type NextRequest, NextResponse } from "next/server";

import { accountCenterContinuationRedirectURL } from "@/lib/auth/account-continuation-url";

export function proxy(request: NextRequest) {
  const redirectURL = accountCenterContinuationRedirectURL(request.url);
  if (!redirectURL) return NextResponse.next();

  const response = NextResponse.redirect(redirectURL, 303);
  response.headers.set("Cache-Control", "private, no-store, max-age=0");
  response.headers.set("Referrer-Policy", "no-referrer");
  return response;
}

export const config = {
  matcher: "/account/login",
};
