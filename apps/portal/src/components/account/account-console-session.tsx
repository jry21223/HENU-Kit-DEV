"use client";

import { createContext, useCallback, useContext, useMemo } from "react";

import { PortalUnauthorizedError } from "@/lib/api/client";
import type { PortalSession } from "@/lib/api/types";

type AccountConsoleSessionContextValue = {
  session: PortalSession;
  requireLogin: () => void;
};

const AccountConsoleSessionContext = createContext<AccountConsoleSessionContextValue | null>(null);

export function AccountConsoleSessionProvider({
  session,
  requireLogin,
  children,
}: {
  session: PortalSession;
  requireLogin: () => void;
  children: React.ReactNode;
}) {
  const value = useMemo(() => ({ session, requireLogin }), [session, requireLogin]);
  return (
    <AccountConsoleSessionContext.Provider value={value}>
      {children}
    </AccountConsoleSessionContext.Provider>
  );
}

/**
 * Account Portfolio presentation may only consume the authenticated Portal
 * Session that the console layout fetched from Portal Gateway. It intentionally
 * has no local-storage or mock fallback.
 */
export function useAccountConsoleSession(): PortalSession {
  const value = useContext(AccountConsoleSessionContext);
  if (!value) {
    throw new Error("Account console requires an authenticated Portal Session");
  }
  return value.session;
}

/**
 * Owner-scoped API responses may discover an expired Portal Session after the
 * layout's initial read. They must enter Login required instead of exposing a
 * misleading retry loop for a permanent 401.
 */
export function useAccountConsoleUnauthorizedHandler(): (error: unknown) => boolean {
  const value = useContext(AccountConsoleSessionContext);
  const requireLogin = value?.requireLogin;
  const handleUnauthorized = useCallback((error: unknown) => {
    if (!(error instanceof PortalUnauthorizedError)) return false;
    if (!requireLogin) {
      throw new Error("Account console requires an authenticated Portal Session");
    }
    requireLogin();
    return true;
  }, [requireLogin]);
  if (!value) {
    throw new Error("Account console requires an authenticated Portal Session");
  }
  return handleUnauthorized;
}
