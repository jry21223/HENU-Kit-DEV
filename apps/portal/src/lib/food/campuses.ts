/**
 * 美食校区静态常量。非 mock 数据：仅校区名/序号，SSR 与客户端一致。
 * 独立模块避免 Account 闭包触及 mock 数据源（check-account-production-boundary）。
 */

export type CampusKey = "minglun" | "jinming" | "longzihu";

export const CAMPUSES: Record<
  CampusKey,
  { name: string; index: string }
> = {
  minglun: { name: "明伦校区", index: "01" },
  jinming: { name: "金明校区", index: "02" },
  longzihu: { name: "龙子湖校区", index: "03" },
};

export const CAMPUS_KEYS = Object.keys(CAMPUSES) as CampusKey[];
