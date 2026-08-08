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

/** One immutable point-ledger fact belonging to the signed-in user. */
export interface AccountPointEntry {
  id: string;
  amount: number;
  reason: string;
  created_at: string;
}

export interface AccountPointsResponse {
  data: {
    balance: number;
    entries: AccountPointEntry[];
    next_cursor: string | null;
  };
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

export type AccountMembershipOrderStatus =
  | "created"
  | "pending_payment"
  | "paid"
  | "closed"
  | "failed"
  | "refunded";

export interface AccountMembershipOrder {
  id: string;
  plan: "lifetime";
  amount_cents: number;
  status: AccountMembershipOrderStatus;
  version: number;
  created_at: string;
  updated_at: string;
}

export interface AccountMembershipOrderResponse {
  data: {
    order: AccountMembershipOrder;
    /**
     * Single-use WeChat payment URI rendered as a QR code. Present only while
     * the order awaits payment and the code is still valid, so an absent value
     * means "no scannable code", never "assume it worked".
     */
    checkout_url?: string;
  };
  request_id: string;
}

export interface AccountMembershipOrdersResponse {
  data: { orders: AccountMembershipOrder[] };
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

export type MaterialType = "note" | "exam" | "mock" | "path" | "lab" | "slides";

/** 转换后的 PPT 单页 */
export interface Slide {
  title: string;
  blocks?: string[];
}

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
  /** 镜像文件在 /materials/ 下的路径;有值时可下载 */
  filePath?: string;
  fileSize?: number;
  /** 已转换的 PPT 页(详情接口返回) */
  slides?: Slide[];
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

/** Browser input for one real QuizCraft session. The API selects questions. */
export interface PortalPracticeSessionInput {
  bank_id: string;
  bank_version_id: string;
  mode: "random" | "difficult" | "chapter" | "favorites";
  chapter_id?: string;
  question_count?: number;
}

/** A server-selected question. It deliberately has no answer key. */
export interface PortalPracticeQuestion {
  question_id: string;
  question_version_id: string;
  type: "single" | "multi" | "judge" | "blank";
  chapter_id: string;
  chapter: string;
  content: string;
  options?: string[];
}

export interface PortalPracticeSessionResponse {
  request_id: string;
  data: {
    session_id: string;
    bank_id: string;
    bank_version_id: string;
    mode: "random" | "difficult" | "chapter" | "favorites";
    excluded_unavailable_count: number;
    questions: PortalPracticeQuestion[];
  };
}

export interface PortalPracticeAnswerInput {
  question_id: string;
  question_version_id: string;
  answer: unknown;
}

export type PracticeFeedbackCategory =
  | "wrong_answer"
  | "ambiguous"
  | "typo"
  | "outdated"
  | "other";

export interface PortalPracticeFeedbackInput {
  bank_id: string;
  question_id: string;
  question_version_id: string;
  category: PracticeFeedbackCategory;
  detail: string;
}

export interface PracticeFeedbackOperation {
  operation_id: string;
  state: string;
  idempotency_key: string;
  request_id: string;
  resource_id: string;
}

export interface PortalPracticeFeedbackResponse {
  request_id: string;
  data: PracticeFeedbackOperation;
}

export interface PracticeFeedbackStatus {
  feedback_id: string;
  bank_id: string;
  question_id: string;
  question_version_id: string;
  category: PracticeFeedbackCategory;
  status: "pending" | "in_progress" | "blocked" | "resolved" | "archived";
  created_at: string;
  updated_at: string;
}

export interface PortalPracticeFeedbackStatusResponse {
  request_id: string;
  data: PracticeFeedbackStatus;
}

export interface FavoriteFolder {
  bank_id: string;
  bank_name: string;
  available_count: number;
  unavailable_count: number;
}

export interface FavoritesOverviewResponse {
  request_id: string;
  data: FavoriteFolder[];
}

export interface FavoriteQuestion {
  bank_id: string;
  question_id: string;
  available: boolean;
  question_version_id?: string;
}

export interface FavoriteListResponse {
  request_id: string;
  data: FavoriteQuestion[];
}

export interface FavoriteWriteResponse {
  request_id: string;
  data: PracticeFeedbackOperation;
}

/** Correctness and answer disclosure arrive only after server-side scoring. */
export interface PortalPracticeAnswerResponse {
  request_id: string;
  data: {
    question_id: string;
    question_version_id: string;
    correct: boolean;
    replayed: boolean;
    expected_answer: unknown;
    analysis: string;
  };
}

/**
 * Dark-until-cutover QuizCraft catalog data. This intentionally stays
 * separate from the legacy Portal API BankSummary shape, which cannot carry
 * the immutable QuizCraft bank-version identifier required to start V2 work.
 */
export interface QuizCraftCatalogBank {
  bank_id: string;
  bank_version_id: string;
  name: string;
  question_count: number;
  available: boolean;
}

export interface QuizCraftCatalogResponse {
  banks: QuizCraftCatalogBank[];
  request_id: string;
}

export type QuizCraftRankingPeriod = "weekly" | "lifetime";

export interface QuizCraftRankingResponse {
  request_id: string;
  data: {
    scope: "overall" | "bank";
    bank_id?: string;
    period: QuizCraftRankingPeriod;
    metric: "correct_answer_count";
    entries: Array<{
      rank: number;
      nickname: string;
      system_avatar: "scholar-blue" | "coder-green" | "reader-amber" | "owl-purple";
      correct_answer_count: number;
    }>;
  };
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

export interface CampusMessage {
  id: string;
  itemId: string;
  author: string;
  time: string;
  text: string;
}

export interface CampusItemDetailResponse {
  item: CampusItem;
  messages: CampusMessage[];
  request_id: string;
}

export interface CategoryListResponse {
  categories: CampusCategory[];
  request_id: string;
}

// ---- Notices (Notice service snapshot) ----

/**
 * Immutable Notice content as published by the Notice service. The service
 * exposes a single bounded snapshot; there is no separate detail endpoint, so
 * the full body travels with the feed and detail expands in place.
 */
export interface NoticeFeedItem {
  id: string;
  source: {
    id: string;
    code: string;
    name: string;
  };
  version: number;
  title: string;
  body: string;
  source_url: string;
  content_hash: string;
  state: "pending_review" | "approved" | "rejected" | "distributed";
  revision: number;
  source_published_at?: string;
  created_at: string;
  distribution_count: number;
  distribution_status?: "queued" | "processing" | "delivered" | "failed";
}

export interface NoticeFeed {
  items: NoticeFeedItem[];
  generated_at: string;
}

/** Gateway envelope mirroring the Notice owner's snapshot contract. */
export interface NoticeFeedEnvelope {
  data: NoticeFeed;
  request_id: string;
}
