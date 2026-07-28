/**
 * Portal Gateway / Portal API types.
 * Aligns with packages/api-contracts/openapi/portal-gateway.yaml
 * and portal-api.yaml (materials, posts, schools, campus).
 */

export type {
  MasterySubject,
  PersonalPracticeStats,
  PersonalPracticeStatsEnvelope,
  PortalSession,
} from "./portal-session.generated";

export interface ErrorEnvelope {
  error: string;
  detail?: string;
  request_id?: string;
}

// ---- Account Portfolio ----

/** A persisted account summary from Portal Gateway, never a session fixture. */
export interface AccountSummary {
  points_balance: number;
  plan: "free" | "lifetime";
  lifetime: boolean;
  unread_notification_count: number;
  open_ticket_count: number;
}

export interface AccountSummaryResponse {
  data: AccountSummary;
  request_id: string;
}

/** The signed-in user's durable membership entitlement. */
export interface AccountMembership {
  plan: "free" | "lifetime";
  lifetime: boolean;
}

export interface AccountMembershipResponse {
  data: AccountMembership;
  request_id: string;
}

export type AccountTicketStatus = "open" | "in_progress" | "resolved";

export interface AccountNotification {
  id: string;
  title: string;
  body: string;
  kind: string;
  ticket_id?: string;
  ticket_reference?: string;
  read_at?: string;
  created_at: string;
}

export interface AccountNotificationsResponse {
  data: {
    notifications: AccountNotification[];
  };
  request_id: string;
}

export interface AccountNotificationResponse {
  data: {
    notification: AccountNotification;
  };
  request_id: string;
}

export interface AccountTicket {
  id: string;
  reference: string;
  title: string;
  category: string;
  status: AccountTicketStatus;
  version: number;
  created_at: string;
  updated_at: string;
}

export interface AccountTicketsResponse {
  data: {
    tickets: AccountTicket[];
  };
  request_id: string;
}

export interface AccountTicketResponse {
  data: {
    ticket: AccountTicket;
  };
  request_id: string;
}

export interface AccountTicketMessage {
  id: string;
  author_kind: "user" | "operator";
  body: string;
  created_at: string;
}

export interface AccountTicketEvent {
  id: string;
  kind: "operator_reply" | "status_transition" | "reopened";
  from_status: AccountTicketStatus;
  to_status: AccountTicketStatus;
  created_at: string;
}

export interface AccountTicketDetailResponse {
  data: {
    ticket: AccountTicket;
    messages: AccountTicketMessage[];
    events: AccountTicketEvent[];
  };
  request_id: string;
}

export interface AccountCreateTicketInput {
  title: string;
  category: string;
  body: string;
}

export interface AccountTicketFollowUpInput {
  body: string;
  expected_version: number;
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
