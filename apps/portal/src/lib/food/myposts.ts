import { apiFetchRequired } from "../api/client";
import type { FoodPostListResponse } from "../api/types";

/**
 * 我的投稿：Gateway 按当前 Portal Session 的 actor 过滤，只返回当前用户
 * 创建过的 Food Post。Food Post 创建即公开，本页只呈现"我发布过什么"，
 * 不存在排队或审核态；401 由 apiFetchRequired 抛 PortalUnauthorizedError。
 */
export function fetchMyFoodPosts(): Promise<FoodPostListResponse> {
  return apiFetchRequired<FoodPostListResponse>("/api/v1/food/posts/mine", {
    cache: "no-store",
    credentials: "include",
  });
}
