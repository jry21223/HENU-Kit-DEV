/**
 * Library gateway adapter.
 * Fetches from API when available, falls back to mock.
 */

import { hasGateway, fetchCourses, fetchCourseMaterials } from "@/lib/api/client";
import type { Material, CourseSummary } from "@/lib/api/types";
import { STATIC_MATERIALS } from "./mock";

let gatewayCourses: CourseSummary[] | null = null;
let gatewayMaterials: Material[] | null = null;

export async function initGateway(): Promise<void> {
  if (!hasGateway) return;
  try {
    const resp = await fetchCourses();
    if (resp) {
      gatewayCourses = resp.courses;
      // Also fetch all materials for detail lookups
      const allMaterials: Material[] = [];
      for (const course of resp.courses) {
        const matResp = await fetchCourseMaterials(course.id);
        if (matResp) allMaterials.push(...matResp.materials);
      }
      gatewayMaterials = allMaterials;
    }
  } catch {
    // silent
  }
}

/** Get all materials (API or mock fallback) */
export function getMaterials(): Material[] {
  return gatewayMaterials ?? STATIC_MATERIALS;
}

/** Get a single material by ID */
export function getMaterialById(id: string): Material | undefined {
  const all = getMaterials();
  return all.find((m) => m.id === id);
}

/** Get courses summary */
export function getCourses(): CourseSummary[] | null {
  return gatewayCourses;
}
