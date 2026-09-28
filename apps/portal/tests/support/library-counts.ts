import type { Page } from "@playwright/test";

/**
 * 首页资料库区块只读分类计数（#555）。计数按 spec 自己的目录夹具算出，首页的数字和
 * /library 列表看到的目录一致。
 */
const MATERIAL_TYPES = ["handout", "exam", "slides", "exercise", "answer", "note", "textbook"] as const;

export function libraryCountsFor(materials: ReadonlyArray<{ type: string }>, requestId = "req_library_counts") {
  return {
    counts: {
      releaseId: "0123456789abcdef0123456789abcdef01234567-0123456789abcdef",
      materialCount: materials.length,
      byType: Object.fromEntries(
        MATERIAL_TYPES.map((type) => [type, materials.filter((material) => material.type === type).length])
      ),
      asOf: "2026-08-11T01:00:00Z",
    },
    request_id: requestId,
  };
}

/** 让首页资料库区块的计数接口按这份目录回应。 */
export async function routeLibraryCounts(page: Page, materials: ReadonlyArray<{ type: string }>) {
  await page.route("**/api/v1/library/material-counts", (route) =>
    route.fulfill({ json: libraryCountsFor(materials) })
  );
}
