import { createContext, useContext } from "react";
import type { ConsoleOverview, ConsoleScope, ConsoleSession } from "./console-gateway";

export type ConsoleAuthState =
  | "loading"
  | "authenticated"
  | "signed_out"
  | "denied"
  | "unavailable";

export type ConsoleOverviewState = "loading" | "ready" | "denied" | "unavailable";

export type NavGroupKey = "workspace" | "business" | "account" | "platform";

export interface ConsoleNavItem {
  key: string;
  path: string;
  label: string;
  group: NavGroupKey;
  permission: string;
  anyPermissions?: string[];
  product?: string;
  headingId: string;
}

export const ALL_CONSOLE_NAV_ITEMS: ConsoleNavItem[] = [
  {
    key: "/",
    path: "/",
    label: "工作台总览",
    group: "workspace",
    permission: "console.overview.read",
    headingId: "overview-heading",
  },
  {
    key: "/food",
    path: "/food",
    label: "美食榜治理",
    group: "business",
    permission: "food.read",
    product: "food",
    headingId: "food-heading",
  },
  {
    key: "/notices",
    path: "/notices",
    label: "校园通知",
    group: "business",
    permission: "notice.read",
    product: "notice",
    headingId: "notice-heading",
  },
  {
    key: "/practice",
    path: "/practice",
    label: "题库管理",
    group: "business",
    permission: "quizcraft.workshop.read",
    product: "quizcraft",
    headingId: "practice-heading",
  },
  {
    key: "/library",
    path: "/library",
    label: "资料库管理",
    group: "business",
    permission: "library.read",
    product: "library",
    headingId: "library-heading",
  },
  {
    key: "/account",
    path: "/account",
    label: "客服工单",
    group: "account",
    permission: "account.tickets.read",
    product: "account-portfolio",
    headingId: "account-tickets-heading",
  },
  {
    key: "/account/memberships",
    path: "/account/memberships",
    label: "会员与订单",
    group: "account",
    permission: "account.membership.write",
    anyPermissions: ["account.membership.write", "account.orders.read"],
    product: "account-portfolio",
    headingId: "account-membership-heading",
  },
  {
    key: "/account/points",
    path: "/account/points",
    label: "积分调整",
    group: "account",
    permission: "account.points.adjust",
    product: "account-portfolio",
    headingId: "account-points-heading",
  },
  {
    key: "/operations",
    path: "/operations",
    label: "平台与安全",
    group: "platform",
    permission: "platform.operations.read",
    headingId: "operations-heading",
  },
];

export const NAV_GROUP_LABELS: Record<NavGroupKey, string> = {
  workspace: "工作台",
  business: "内容与业务",
  account: "用户与客服",
  platform: "平台与安全",
};

export function hasConsoleAccess(
  permissions: string[],
  scopes: ConsoleScope[],
  permission: string,
  product?: string,
): boolean {
  if (!permissions.includes(permission)) return false;
  return scopes.some((scope) =>
    product
      ? scope.kind === "product" && scope.product_code === product
      : scope.kind === "platform",
  );
}

export interface ConsoleContextValue {
  authState: ConsoleAuthState;
  session?: ConsoleSession;
  operatorId?: string;
  permissions: string[];
  scopes: ConsoleScope[];
  principalGeneration: number;
  sessionExpired: boolean;
  overview?: ConsoleOverview;
  overviewState: ConsoleOverviewState;
  authorizedNav: ConsoleNavItem[];
  refreshSession: () => Promise<void>;
  refreshOverview: () => Promise<void>;
  requestSignOut: (kind: "signout" | "switch_account") => void;
}

export const ConsoleContext = createContext<ConsoleContextValue | null>(null);

export function useConsole(): ConsoleContextValue {
  const ctx = useContext(ConsoleContext);
  if (!ctx) throw new Error("useConsole must be used inside ConsoleContext.Provider");
  return ctx;
}
