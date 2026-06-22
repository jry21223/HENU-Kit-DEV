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
