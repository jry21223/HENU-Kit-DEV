import { notFound } from "next/navigation";
import { UserProfileView } from "@/components/user/user-profile";
import { SiteShell } from "@/components/layout/site-shell";
import { getApi, type UserProfileResponse } from "@/lib/api";

type PageProps = {
  params: Promise<{ id: string }>;
};

const copy = {
  unavailable: "\u7528\u6237\u4e3b\u9875\u6682\u65f6\u4e0d\u53ef\u7528\u3002",
};

async function loadProfile(id: string) {
  try {
    const response = await getApi<UserProfileResponse>(`/users/${encodeURIComponent(id)}`);
    return { error: "", profile: response.data };
  } catch (error) {
    const message = error instanceof Error ? error.message : copy.unavailable;
    return { error: message, profile: null };
  }
}

export default async function UserPage({ params }: PageProps) {
  const { id } = await params;
  const { error, profile } = await loadProfile(id);

  if (!profile && error.includes("404")) {
    notFound();
  }

  return (
    <SiteShell>
      {profile ? <UserProfileView initialProfile={profile} userId={id} /> : <p className="rounded-2xl border border-border bg-card p-4 text-sm text-muted-foreground">{error || copy.unavailable}</p>}
    </SiteShell>
  );
}
