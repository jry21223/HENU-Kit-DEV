/**
 * Portal Gateway / Portal API types.
 * Aligns with packages/api-contracts/openapi/portal-gateway.yaml
 * and portal-api.yaml (materials, posts, schools, campus).
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

// ---- Library (gateway courses + portal-api materials) ----

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

export type MaterialType = "note" | "exam" | "mock" | "path" | "lab";

export interface Material {
  id: string;
  type: MaterialType;
  subject: string;
  title: string;
  author: string;
  intro: string;
  toc: string[];
  pages: string[][];
  price: number;
  previewPages: number;
  rating: number;
  downloads: number;
  favs: number;
}

export interface MaterialListResponse {
  materials: Material[];
  request_id: string;
}

export interface MaterialDetailResponse {
  material: Material;
  request_id: string;
}

// ---- Food ----

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

export interface PostBlock {
  type: "h2" | "p" | "quote" | "list" | "img";
  text?: string;
  items?: string[];
  src?: string;
  ref?: number;
}

export interface Shop {
  name: string;
  lat: number;
  lng: number;
}

export type CampusKey = "minglun" | "jinming" | "longzihu";

export interface FoodPost {
  id: string;
  campus: CampusKey;
  title: string;
  excerpt: string;
  blocks: PostBlock[];
  author: string;
  likes: number;
  stars: number;
  tags: string[];
  shop: Shop;
  time: string;
  hidden: boolean;
  images?: string[];
}

export interface FoodComment {
  id: string;
  postId: string;
  author: string;
  time: string;
  text: string;
}

export interface FoodPostListResponse {
  posts: FoodPost[];
  request_id: string;
}

export interface FoodPostDetailResponse {
  post: FoodPost;
  comments: FoodComment[];
  request_id: string;
}

// ---- Practice ----

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

export interface QuizListMeta {
  id: string;
  name: string;
  creator: string;
  tags: string[];
  poolKey: string;
  count: number;
  completion: number;
}

export interface PracticeSubject {
  id: string;
  name: string;
  lists: QuizListMeta[];
}

export interface PracticeMajor {
  id: string;
  name: string;
  subjects: PracticeSubject[];
}

export interface PracticeSchool {
  id: string;
  name: string;
  majors: PracticeMajor[];
}

export interface SchoolListResponse {
  schools: PracticeSchool[];
  request_id: string;
}

// ---- Campus ----

export type CampusItemType = "help" | "sell";
export type CampusItemStatus = "open" | "ongoing" | "done" | "hidden";

export interface CampusCategory {
  key: string;
  name: string;
  code: string;
}

export interface CampusItem {
  id: string;
  type: CampusItemType;
  category: string;
  title: string;
  desc: string;
  price: number;
  seller: string;
  credit: number;
  dealsDone: number;
  wants: number;
  place: string;
  deadline?: string;
  status: CampusItemStatus;
  time: string;
  images?: string[];
}

export interface CampusItemListResponse {
  items: CampusItem[];
  request_id: string;
}

export interface CategoryListResponse {
  categories: CampusCategory[];
  request_id: string;
}

// ---- Notices ----

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
