const baseUrl = import.meta.env.VITE_API_BASE_URL ?? "http://localhost:8080/api/v1";
const tokenKey = "final-review-admin-token";

export type Envelope<T> = {
  code: number;
  message: string;
  data?: T;
  details?: unknown;
};

export type User = {
  id: string;
  email: string;
  name: string;
  role: string;
  status: string;
  schoolId?: string;
  majorId?: string;
  grade?: string;
  emailVerified: boolean;
  frozenUntil?: string;
  pointsBalance?: number;
  createdAt?: string;
  updatedAt?: string;
};

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

export type School = {
  id: string;
  name: string;
  slug: string;
  status: string;
};

export type College = {
  id: string;
  schoolId: string;
  name: string;
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
  accessLevel: string;
  status: string;
  reviewerId?: string;
  reviewedAt?: string;
  reviewReason?: string;
};

export type DownloadRecord = {
  id: string;
  userId?: string;
  materialId: string;
  accessLevel: string;
  ip?: string;
  userAgent?: string;
  downloadedAt: string;
  material?: Material;
};

export type OperationLog = {
  id: string;
  operatorId: string;
  action: string;
  targetType: string;
  targetId: string;
  ip?: string;
  userAgent?: string;
  metadata?: unknown;
  createdAt: string;
  updatedAt: string;
};

export type AITask = {
  id: string;
  userId?: string;
  courseId?: string;
  type: string;
  status: string;
  input?: unknown;
  result?: unknown;
  error: string;
  startedAt?: string;
  endedAt?: string;
  createdAt: string;
  updatedAt: string;
};

export type AIDraft = {
  id: string;
  taskId: string;
  courseId?: string;
  outputType: string;
  status: string;
  reviewerId?: string;
  reviewedAt?: string;
  reviewReason?: string;
  draftContent?: unknown;
  publishedId?: string;
  createdAt: string;
  updatedAt: string;
};

export type BlogPost = {
  id: string;
  authorId: string;
  title: string;
  slug: string;
  content: string;
  status: string;
  reviewerId?: string;
  reviewedAt?: string;
  reviewReason?: string;
  visibility?: string;
  likeCount?: number;
  commentCount?: number;
  collectCount?: number;
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
  status: string;
  reviewerId?: string;
  reviewedAt?: string;
  reviewReason?: string;
  visibility?: string;
  likeCount?: number;
  commentCount?: number;
  collectCount?: number;
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
  currentTitle?: string;
  currentContent?: string;
  currentVersion?: number;
  currentStatus?: string;
  baseContent?: string;
  baseSummary?: string;
  isStale?: boolean;
  createdAt: string;
  updatedAt: string;
};

export type ForumPost = {
  id: string;
  authorId: string;
  boardId: string;
  title: string;
  content: string;
  type: string;
  rewardPoints?: number;
  rewardStatus?: string;
  status: string;
  reviewerId?: string;
  reviewedAt?: string;
  reviewReason?: string;
  visibility?: string;
  likeCount?: number;
  commentCount?: number;
  collectCount?: number;
  createdAt: string;
  updatedAt: string;
};

export type ForumReply = {
  id: string;
  authorId: string;
  postId: string;
  content: string;
  isBest: boolean;
  status: string;
  reviewerId?: string;
  reviewedAt?: string;
  reviewReason?: string;
  createdAt: string;
  updatedAt: string;
};

export type Report = {
  id: string;
  reporterId: string;
  targetType: string;
  targetId: string;
  reason: string;
  description: string;
  status: string;
  reviewerId?: string;
  reviewedAt?: string;
  reviewReason?: string;
  createdAt: string;
  updatedAt: string;
};

export type AnalyticsOverview = {
  totals: {
    users: number;
    courses: number;
    materials: number;
    publishedMaterials: number;
    pendingMaterials: number;
    packages: number;
    downloads: number;
    reports: number;
    pendingReports: number;
  };
  downloadTrend: Array<{ date: string; count: number }>;
  topMaterials: Array<{
    materialId: string;
    title: string;
    courseId: string;
    type: string;
    accessLevel: string;
    status: string;
    downloads: number;
  }>;
  courseDemand: Array<{
    courseId: string;
    courseName: string;
    grade: string;
    status: string;
    materialCount: number;
    publishedMaterialCount: number;
    downloadCount: number;
  }>;
  accessBreakdown: Array<{ accessLevel: string; downloads: number }>;
  reportBreakdown: Array<{ targetType: string; status: string; count: number }>;
};

export type LoginData = {
  user: User;
  accessToken: string;
  tokenType: string;
  expiresAt: string;
};

export function getStoredToken() {
  return localStorage.getItem(tokenKey) ?? "";
}

export function setStoredToken(token: string) {
  if (token) {
    localStorage.setItem(tokenKey, token);
  } else {
    localStorage.removeItem(tokenKey);
  }
}

export function apiUrl(path: string) {
  return `${baseUrl}${path}`;
}

export async function apiRequest<T>(path: string, init: RequestInit = {}): Promise<Envelope<T>> {
  const token = getStoredToken();
  const headers = new Headers(init.headers);
  if (!(init.body instanceof FormData)) {
    headers.set("Content-Type", "application/json");
  }
  if (token) {
    headers.set("Authorization", `Bearer ${token}`);
  }

  const response = await fetch(`${baseUrl}${path}`, {
    ...init,
    headers,
    credentials: "include",
  });
  const payload = (await response.json().catch(() => ({}))) as Envelope<T>;
  if (!response.ok || payload.code !== 0) {
    throw new Error(payload.message || `API request failed with ${response.status}`);
  }
  return payload;
}

export async function sendCode(email: string) {
  return apiRequest<{ expiresInSeconds: number; devCode?: string }>("/auth/send-code", {
    method: "POST",
    body: JSON.stringify({ email }),
  });
}

export async function login(email: string, code: string, name: string) {
  const response = await apiRequest<LoginData>("/auth/login", {
    method: "POST",
    body: JSON.stringify({ email, code, name }),
  });
  if (response.data?.accessToken) {
    setStoredToken(response.data.accessToken);
  }
  return response;
}

export async function logout() {
  try {
    await apiRequest<{ ok: boolean }>("/auth/logout", { method: "POST" });
  } finally {
    setStoredToken("");
  }
}
