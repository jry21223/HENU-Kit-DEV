/**
 * Portal API response types.
 * Matches packages/api-contracts/openapi/portal-api.yaml
 */

export interface PortalSession {
  user_id: string;
  expires_at: string;
}

export interface ErrorEnvelope {
  error: string;
  detail?: string;
  request_id?: string;
}

// ─── Library ───────────────────────────────────────────────

export interface Material {
  id: string;
  type: "note" | "exam" | "mock" | "path" | "lab";
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

export interface LibraryCoursesResponse {
  materials: Material[];
  request_id: string;
}

// ─── Food ──────────────────────────────────────────────────

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

export interface FoodPost {
  id: string;
  campus: string;
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

export interface FoodVenuesResponse {
  posts: FoodPost[];
  request_id: string;
}

// ─── Practice ──────────────────────────────────────────────

export interface Question {
  id: string;
  subject: string;
  chapter: string;
  difficulty: number;
  stem: string;
  options: [string, string, string, string];
  answer: number;
  explanation: string;
  accuracy: number;
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

export interface Subject {
  id: string;
  name: string;
  lists: QuizListMeta[];
}

export interface Major {
  id: string;
  name: string;
  subjects: Subject[];
}

export interface School {
  id: string;
  name: string;
  majors: Major[];
}

export interface PracticeBanksResponse {
  schools: School[];
  request_id: string;
}

export interface LeaderboardRow {
  name: string;
  questions: number;
  accuracy: number;
  streak: number;
  isYou?: boolean;
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
