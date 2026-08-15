import {
  PortalUnauthorizedError,
  fetchAccountMembership,
  fetchSession,
  formatPortalError,
} from "../api/client";
import type {
  AccountMembershipResponse,
  CareerProfile,
  CareerSearch,
} from "../api/types";
import {
  isCareerLifetimeRequiredError,
  loadCareerData,
  requestCareerSearches,
} from "./gateway";

/**
 * /career 页面认证与会员门分支（#400）。
 *
 * 会话与会员读取只认 Portal Gateway：fetchSession 为空即匿名，
 * membership 非 lifetime 即免费；lifetime 后再读求职画像与历史。
 * 任一环节失败都诚实进入 error，绝不回退本地 mock 成功。
 */
export type CareerViewState =
  | { kind: "anonymous" }
  | { kind: "free" }
  | { kind: "lifetime-no-profile" }
  | { kind: "lifetime-ready"; profile: CareerProfile; searches: CareerSearch[] }
  | { kind: "error"; message: string };

/**
 * /career/history 历史页分支（#402）：门与主页面一致，
 * 但历史列表实时读取，不展示轮询缓存里的旧快照。
 */
export type CareerHistoryViewState =
  | { kind: "anonymous" }
  | { kind: "free" }
  | { kind: "lifetime-no-profile" }
  | { kind: "ready"; searches: CareerSearch[] }
  | { kind: "error"; message: string };

type CareerGateState =
  | { kind: "anonymous" }
  | { kind: "free" }
  | { kind: "lifetime" }
  | { kind: "error"; message: string };

async function resolveCareerGate(): Promise<CareerGateState> {
  let session: Awaited<ReturnType<typeof fetchSession>>;
  try {
    session = await fetchSession();
  } catch (error) {
    return { kind: "error", message: formatPortalError(error) };
  }
  if (!session) return { kind: "anonymous" };

  let membership: AccountMembershipResponse;
  try {
    membership = await fetchAccountMembership();
  } catch (error) {
    if (error instanceof PortalUnauthorizedError) return { kind: "anonymous" };
    return { kind: "error", message: formatPortalError(error) };
  }

  if (membership.data.plan !== "lifetime" || !membership.data.lifetime) {
    return { kind: "free" };
  }
  return { kind: "lifetime" };
}

/** 画像是否足以发起扫描：目标岗位必填。 */
export function isCareerProfileReady(profile: CareerProfile): boolean {
  return (
    typeof profile.target_roles === "string" &&
    profile.target_roles.trim().length > 0
  );
}

export async function resolveCareerView(): Promise<CareerViewState> {
  const gate = await resolveCareerGate();
  if (gate.kind !== "lifetime") return gate;

  const career = await loadCareerData();
  if (career.error) return { kind: "error", message: career.error };
  if (!career.profile || !isCareerProfileReady(career.profile)) {
    return { kind: "lifetime-no-profile" };
  }
  return {
    kind: "lifetime-ready",
    profile: career.profile,
    searches: career.searches,
  };
}

export async function resolveCareerHistoryView(): Promise<CareerHistoryViewState> {
  const gate = await resolveCareerGate();
  if (gate.kind !== "lifetime") return gate;

  const career = await loadCareerData();
  if (career.error) return { kind: "error", message: career.error };
  if (!career.profile || !isCareerProfileReady(career.profile)) {
    return { kind: "lifetime-no-profile" };
  }

  try {
    const list = await requestCareerSearches();
    return { kind: "ready", searches: list.searches };
  } catch (error) {
    if (isCareerLifetimeRequiredError(error)) {
      return { kind: "free" };
    }
    return { kind: "error", message: formatPortalError(error) };
  }
}
