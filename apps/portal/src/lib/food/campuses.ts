/**
 * 美食校区静态常量。非 mock 数据：仅校区名/序号/坐标，SSR 与客户端一致。
 * 独立模块避免 Account 闭包触及 mock 数据源（check-account-production-boundary）。
 */

export type CampusKey = "minglun" | "jinming" | "longzihu";

export const CAMPUSES: Record<
  CampusKey,
  { name: string; index: string; lat: number; lng: number }
> = {
  minglun: { name: "明伦校区", index: "01", lat: 34.8186, lng: 114.3544 },
  jinming: { name: "金明校区", index: "02", lat: 34.8225, lng: 114.3079 },
  longzihu: { name: "龙子湖校区", index: "03", lat: 34.8173, lng: 113.8293 },
};

export const CAMPUS_KEYS = Object.keys(CAMPUSES) as CampusKey[];
