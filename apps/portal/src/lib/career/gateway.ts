/**
 * Career gateway adapter.
 *
 * 生产 / require-gateway：必须走 API；失败不静默回退 mock。
 * mock 仅本地开发且 NEXT_PUBLIC_PORTAL_ALLOW_MOCK=1 时提供最小占位。
 */

import {
  PortalConfigError,
  PortalForbiddenError,
  PortalHttpError,
  createCareerSearch,
  formatPortalError,
  getCareerProfile,
  getCareerSearchStatus,
  hasGateway,
  listCareerSearches,
  mockAllowed,
  updateCareerProfile,
} from "../api/client";
import type {
  CareerProfile,
  CareerProfileInput,
  CareerProfileResponse,
  CareerSearch,
  CareerSearchResponse,
  CareerSearchesResponse,
} from "../api/types";

// ---- Lifetime 会员门 ----

/** 网关 403 envelope 的 error 码：非 Lifetime 会员访问求职雷达。 */
export const CAREER_LIFETIME_REQUIRED_CODE = "lifetime_required";

/** 是否命中 Lifetime 会员门；UI 可据此分支引导开通会员。 */
export function isCareerLifetimeRequiredError(error: unknown): boolean {
  return (
    error instanceof PortalForbiddenError &&
    error.errorCode === CAREER_LIFETIME_REQUIRED_CODE
  );
}

/** 命中 Lifetime 门时的引导文案，直接展示给用户。 */
export function careerLifetimeRequiredMessage(): string {
  return "求职雷达需要 Lifetime VIP 会员，开通后即可使用";
}

export function careerSearchCreateErrorMessage(error: unknown): string {
  if (error instanceof PortalHttpError) {
    if (error.errorCode === "SEARCH_ALREADY_ACTIVE") {
      return "已有扫描任务正在进行，请等待完成后再试。";
    }
    if (error.errorCode === "SEARCH_RATE_LIMITED") {
      return "本小时扫描次数已用完，请稍后再试。";
    }
  }
  return formatPortalError(error);
}

// ---- mock 最小占位 ----

/** 空画像占位（仅 ALLOW_MOCK=1 且无 gateway 时生效）。 */
export const EMPTY_CAREER_PROFILE: CareerProfile = {
  user_id: "",
  target_roles: "",
  tech_stack: "",
  locations: "",
  job_type: "",
  graduation_year: null,
  resume_text: "",
  email_notification_enabled: false,
  updated_at: "",
};

// ---- 读缓存 ----

let profileCache: CareerProfile | null = null;
let searchesCache: CareerSearch[] | null = null;
let lastError: unknown = null;
let loaded = false;

export async function initCareerGateway(): Promise<void> {
  if (loaded) return;

  if (!hasGateway) {
    if (mockAllowed) {
      loaded = true;
      lastError = null;
      return;
    }
    lastError = new PortalConfigError("服务未就绪，请联系维护者。");
    return;
  }

  try {
    const [profileResp, searchesResp] = await Promise.all([
      getCareerProfile(),
      listCareerSearches(),
    ]);
    profileCache = profileResp.profile;
    searchesCache = searchesResp.searches;
    loaded = true;
    lastError = null;
  } catch (e) {
    lastError = e;
    if (!mockAllowed) {
      profileCache = null;
      searchesCache = null;
      loaded = false;
    } else {
      loaded = true;
    }
  }
}

export function getCareerProfileData(): CareerProfile | null {
  return profileCache;
}

export function getCareerSearches(): CareerSearch[] | null {
  return searchesCache;
}

export function isCareerReady(): boolean {
  return loaded || mockAllowed;
}

export interface CareerDataResult {
  profile: CareerProfile | null;
  searches: CareerSearch[];
  error: string | null;
}

/**
 * 求职雷达统一加载入口（/career 页面共用）。
 *
 * 走 gateway 的 mock/live 决策：gateway 有缓存直接返回；
 * 失败时 mock 允许则返回空占位，否则返回 formatPortalError 文案。
 */
export async function loadCareerData(): Promise<CareerDataResult> {
  await initCareerGateway();

  const profile = getCareerProfileData();
  const searches = getCareerSearches();
  if (profile && searches) {
    return { profile, searches, error: null };
  }

  if (mockAllowed) {
    return { profile: EMPTY_CAREER_PROFILE, searches: [], error: null };
  }

  // 走到这里时 initCareerGateway 必然已记录错误（无 gateway 或拉取失败），
  // ?? 仅为类型兜底；Lifetime 门单独成句，其余统一走 formatPortalError。
  const err = lastError ?? new Error("加载求职雷达数据失败");
  return {
    profile: null,
    searches: [],
    error: isCareerLifetimeRequiredError(err)
      ? careerLifetimeRequiredMessage()
      : formatPortalError(err),
  };
}

// ---- 命令封装 ----

/**
 * 创建搜索（带 Idempotency-Key）。mock 允许时返回最小占位伪搜索；
 * 生产必须真实提交，非 Lifetime → isCareerLifetimeRequiredError。
 */
export async function requestCareerSearch(
  profile: CareerProfileInput,
  idempotencyKey: string
): Promise<CareerSearchResponse> {
  if (!hasGateway && mockAllowed) {
    return {
      search: {
        id: `mock-search-${idempotencyKey}`,
        status: "queued",
        user_id: "",
        has_email: false,
        created_at: new Date().toISOString(),
      },
      request_id: "mock-career-search",
    };
  }
  return createCareerSearch(profile, idempotencyKey);
}

/** 轮询单次搜索状态。 */
export async function requestCareerSearchStatus(
  searchID: string
): Promise<CareerSearchResponse> {
  if (!hasGateway && mockAllowed) {
    return {
      search: {
        id: searchID,
        status: "queued",
        user_id: "",
        has_email: false,
        created_at: new Date().toISOString(),
      },
      request_id: "mock-career-status",
    };
  }
  return getCareerSearchStatus(searchID);
}

/** 实时读取当前用户的历史搜索（不走读缓存，供历史页与重开恢复用）。 */
export async function requestCareerSearches(): Promise<CareerSearchesResponse> {
  if (!hasGateway && mockAllowed) {
    return { searches: [], request_id: "mock-career-searches" };
  }
  return listCareerSearches();
}

/** 全量替换求职画像。 */
export async function requestCareerProfileUpdate(
  profile: CareerProfileInput
): Promise<CareerProfileResponse> {
  if (!hasGateway && mockAllowed) {
    return {
      profile: { ...EMPTY_CAREER_PROFILE, ...profile },
      request_id: "mock-career-profile",
    };
  }
  return updateCareerProfile(profile);
}
