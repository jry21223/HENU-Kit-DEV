/**
 * Portal Gateway API types.
 * Matches packages/api-contracts/openapi/portal-gateway.yaml
 */

export interface PortalSession {
  user_id: string;
  expires_at: string; // ISO 8601
}

export interface ErrorEnvelope {
  error: string;
  detail?: string;
  request_id?: string;
}

export interface CourseSummary {
  id: string;
  name: string;
  subject: string;
  material_count: number;
}

export interface LibraryCoursesResponse {
  courses: CourseSummary[];
  request_id: string;
}

export interface VenueSummary {
  id: string;
  name: string;
  rating: number;
  tier: string;
  campus: string;
}

export interface FoodVenuesResponse {
  campus: string;
  venues: VenueSummary[];
  request_id: string;
}

export interface BankSummary {
  id: string;
  name: string;
  subject: string;
  question_count: number;
}

export interface PracticeBanksResponse {
  banks: BankSummary[];
  request_id: string;
}

export interface NoticeSummary {
  id: string;
  title: string;
  source: string;
  published_at: string;
}

export interface NoticeListResponse {
  notices: NoticeSummary[];
  request_id: string;
}
