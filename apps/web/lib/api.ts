const baseUrl = process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080/api/v1";

export type ApiEnvelope<T> = {
  code: number;
  message: string;
  data: T;
};

export async function getApi<T>(path: string): Promise<ApiEnvelope<T>> {
  const response = await fetch(`${baseUrl}${path}`, {
    next: { revalidate: 10 },
  });

  if (!response.ok) {
    throw new Error(`API request failed with ${response.status}`);
  }

  return response.json() as Promise<ApiEnvelope<T>>;
}

export async function postApi<T>(path: string, body: unknown): Promise<ApiEnvelope<T>> {
  const response = await fetch(`${baseUrl}${path}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });

  if (!response.ok) {
    throw new Error(`API request failed with ${response.status}`);
  }

  return response.json() as Promise<ApiEnvelope<T>>;
}

export type Course = {
  id: string;
  schoolId: string;
  collegeId: string;
  majorId: string;
  grade: string;
  name: string;
  slug: string;
  description: string;
  examScope: string;
  status: string;
};

export type User = {
  id: string;
  email: string;
  name: string;
  role: string;
  status: string;
  schoolId?: string | null;
  majorId?: string | null;
  grade: string;
  emailVerified: boolean;
};

export type School = {
  id: string;
  name: string;
  slug: string;
  status: string;
};

export type Major = {
  id: string;
  schoolId: string;
  collegeId: string;
  name: string;
  slug: string;
  status: string;
};

export type Material = {
  id: string;
  courseId: string;
  title: string;
  type: string;
  description: string;
  fileName: string;
  fileSize: number;
  previewContent: string;
  accessLevel: "free" | "login_required" | "paid" | "member_only";
  status: string;
  updatedAt?: string;
};

export type CoursePackage = {
  id: string;
  schoolId: string;
  collegeId: string;
  majorId: string;
  courseId?: string;
  grade: string;
  title: string;
  slug: string;
  description: string;
  priceFen: number;
  currency: string;
  status: string;
};

export type CoursePackageItem = {
  id: string;
  packageId: string;
  resourceType: string;
  resourceId: string;
  sortOrder: number;
};

export type CoursePackageDetail = {
  package: CoursePackage;
  items: CoursePackageItem[];
  materials: Material[];
};

export type MaterialAccessGrant = {
  id: string;
  userId: string;
  materialId?: string;
  packageId?: string;
  source: string;
  expiresAt?: string;
};

export type EntitlementSummary = {
  directMaterialGrants: number;
  packageGrants: number;
  unlockedMaterials: number;
};

export type MaterialEntitlement = {
  grant: MaterialAccessGrant;
  material?: Material;
};

export type PackageEntitlement = {
  grant: MaterialAccessGrant;
  package?: CoursePackage;
  materials: Material[];
};

export type Entitlements = {
  summary: EntitlementSummary;
  materialGrants: MaterialEntitlement[];
  packageGrants: PackageEntitlement[];
};

export type Order = {
  id: string;
  userId: string;
  productType: string;
  productId: string;
  outTradeNo: string;
  paymentProvider: string;
  status: string;
  amountTotal: number;
  currency: string;
  paidAt?: string;
  riskFlag?: string;
};

export type OrderCreateResult = {
  order?: Order;
  package?: CoursePackage;
  alreadyOwned: boolean;
  alreadyPending: boolean;
  entitlementGranted: boolean;
};

export type OrderStatus = {
  orderId: string;
  status: string;
  paidAt?: string;
  entitlementGranted: boolean;
  paymentProvider: string;
  productType: string;
  productId: string;
  amountTotal: number;
  currency: string;
  packageTitle?: string;
};

export type MaterialDownload = {
  id: string;
  materialId: string;
  accessLevel: Material["accessLevel"];
  downloadedAt: string;
  material?: Material;
};

export type QuizOption = {
  id: string;
  label: string;
  content: string;
  sortOrder: number;
};

export type QuizQuestion = {
  id: string;
  courseId: string;
  type: string;
  stem: string;
  difficulty: number;
  options?: QuizOption[];
};

export type WrongQuestion = {
  id: string;
  userId: string;
  questionId: string;
  courseId: string;
  wrongCount: number;
  lastAnswer: string;
  createdAt: string;
  updatedAt: string;
};

export type WeaknessPoint = {
  courseId: string;
  wrongCount: number;
};

export type ForumBoard = {
  id: string;
  name: string;
  slug: string;
  description: string;
  status: string;
};

export type ForumPost = {
  id: string;
  authorId: string;
  boardId: string;
  title: string;
  content: string;
  type: "normal" | "question" | "reward";
  rewardPoints: number;
  rewardStatus?: string;
  visibility: string;
  likeCount: number;
  commentCount: number;
  collectCount: number;
  createdAt: string;
  updatedAt: string;
};

export type ForumReply = {
  id: string;
  authorId: string;
  postId: string;
  content: string;
  isBest: boolean;
  createdAt: string;
  updatedAt: string;
};

export type WikiEntry = {
  id: string;
  authorId: string;
  courseId?: string;
  title: string;
  slug: string;
  content: string;
  version: number;
  visibility: string;
  likeCount: number;
  commentCount: number;
  collectCount: number;
  createdAt: string;
  updatedAt: string;
};

export type WikiEditProposal = {
  id: string;
  entryId: string;
  editorId: string;
  baseVersion: number;
  proposedTitle: string;
  proposedContent: string;
  summary: string;
  status: string;
  reviewerId?: string;
  reviewedAt?: string;
  reviewReason?: string;
  createdAt: string;
  updatedAt: string;
};

export type BlogPost = {
  id: string;
  authorId: string;
  title: string;
  slug: string;
  content: string;
  visibility: string;
  likeCount: number;
  commentCount: number;
  collectCount: number;
  createdAt: string;
  updatedAt: string;
};

export type MyForumPost = {
  id: string;
  boardId: string;
  title: string;
  content: string;
  type: ForumPost["type"];
  rewardPoints: number;
  rewardStatus?: string;
  status: string;
  reviewReason?: string;
  visibility: string;
  commentCount: number;
  createdAt: string;
  updatedAt: string;
};

export type MyForumReply = {
  id: string;
  postId: string;
  postTitle: string;
  postStatus: string;
  content: string;
  isBest: boolean;
  status: string;
  reviewReason?: string;
  createdAt: string;
  updatedAt: string;
};

export type NotificationItem = {
  id: string;
  userId: string;
  type: string;
  title: string;
  body: string;
  data?: Record<string, unknown>;
  readAt?: string | null;
  createdAt: string;
  updatedAt: string;
};

export function apiBaseUrl() {
  return baseUrl;
}
