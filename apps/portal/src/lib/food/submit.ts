import {
  apiFetchRequired,
  PortalApiError,
  PortalHttpError,
} from "../api/client";
import type { FoodPost } from "../api/types";
import { fileToDataUrl } from "../image";

/**
 * 站内美食投稿 create 命令(#387)。
 *
 * POST /api/v1/food/posts 由 Portal Gateway 绑定当前 Portal Session 的
 * actor;创建即公开、无审核态。浏览器侧只负责表单校验与幂等 key 保留。
 */

// ---- 输入 / 响应类型(锁定契约 §5,本地定义,不改 types.ts)----

export type FoodPostCampus = "minglun" | "jinming" | "longzihu";

/** 五档定位 wire key(与 FOOD_TIERS 的 tier key 一一对应)。 */
export type FoodPostTierKey = "hang" | "top" | "elite" | "npc" | "bad";

export interface FoodPostDishInput {
  /** 菜名必填;价格与理由可留空。 */
  name: string;
  price?: string;
  reason?: string;
}

export interface FoodPostImageInput {
  content_type: "image/jpeg" | "image/png" | "image/webp";
  /** base64 内容,不含 data: 前缀。 */
  data: string;
}

export interface FoodPostCreateInput {
  venue_name: string;
  campus: FoodPostCampus;
  tier: FoodPostTierKey;
  review_text: string;
  price_reference?: string;
  hours_reference?: string;
  dishes?: FoodPostDishInput[];
  images?: FoodPostImageInput[];
}

/** Gateway 解包后的创建响应:envelope data 展开为 {post, request_id}。 */
export interface FoodPostCreateResponse {
  post: FoodPost;
  request_id: string;
}

// ---- 每日上限 ----

/** Food 每日投稿上限错误码(food.yaml 429 响应,经 Gateway 原样透传)。 */
export const FOOD_POST_DAILY_CAP_CODE = "DAILY_POST_CAP_REACHED";

/** 每日上限的中文提示,直接展示给用户。 */
export function foodPostDailyCapMessage(): string {
  return "今天已经投满 3 条，明天再来吧";
}

/**
 * 是否命中每日投稿上限。
 *
 * 共享 client.ts 的 parseErrorBody 不保留 Food error envelope 的
 * error.code(只把 message 交给 PortalHttpError),而 POST /api/v1/food/posts
 * 的 429 在 food.yaml 中唯一语义即每日上限,因此按 status 判定。
 * client.ts 是锁定文件,本模块不改它。
 */
export function isFoodPostDailyCapError(error: unknown): boolean {
  return error instanceof PortalHttpError && error.status === 429;
}

// ---- create 命令 ----

function foodPostCommandInit(
  idempotencyKey: string,
  input: FoodPostCreateInput
): RequestInit {
  const key = idempotencyKey.trim();
  if (key.length < 8 || key.length > 200) {
    throw new PortalApiError("Invalid Food Post idempotency key", {
      code: "PORTAL_INVALID_FOOD_POST_IDEMPOTENCY_KEY",
    });
  }
  return {
    method: "POST",
    cache: "no-store",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Idempotency-Key": key,
    },
    body: JSON.stringify(input),
  };
}

/**
 * 创建一条 Food Post(创建即公开)。调用方必须保留 idempotencyKey 供
 * 双击/重试复用;失败按 PortalApiError 体系抛出:
 * 401 → PortalUnauthorizedError,429(每日上限)→ PortalHttpError。
 */
export async function createFoodPost(
  input: FoodPostCreateInput,
  idempotencyKey: string
): Promise<FoodPostCreateResponse> {
  return apiFetchRequired<FoodPostCreateResponse>(
    "/api/v1/food/posts",
    foodPostCommandInit(idempotencyKey, input)
  );
}

// ---- 图片转换 ----

/**
 * File → {content_type, data}:复用 fileToDataUrl 的类型/大小预检
 * (JPG/PNG/WebP、单张 ≤2MiB),再从 dataURL 去掉 data: 前缀得到纯 base64。
 */
export async function foodPostImageFromFile(
  file: File
): Promise<FoodPostImageInput> {
  const dataUrl = await fileToDataUrl(file);
  const comma = dataUrl.indexOf(",");
  const header = comma >= 0 ? dataUrl.slice(0, comma) : "";
  const contentType = (header.match(/^data:([^;]+)/)?.[1] ??
    file.type) as FoodPostImageInput["content_type"];
  return {
    content_type: contentType,
    data: dataUrl.slice(comma + 1),
  };
}
