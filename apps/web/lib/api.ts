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

export function apiBaseUrl() {
  return baseUrl;
}
