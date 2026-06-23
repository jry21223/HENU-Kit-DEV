"use client";

import Link from "next/link";
import { FormEvent, useEffect, useMemo, useState } from "react";
import { apiBaseUrl, type Entitlements, type Major, type School, type User } from "@/lib/api";

type Envelope<T> = {
  code: number;
  message: string;
  data?: T;
};

type ProfileFormState = {
  name: string;
  schoolId: string;
  majorId: string;
  grade: string;
};

const copy = {
  loading: "\u6b63\u5728\u8bfb\u53d6\u767b\u5f55\u72b6\u6001...",
  loginRequired: "\u9700\u8981\u5148\u767b\u5f55\u5b66\u751f\u90ae\u7bb1\u540e\u624d\u80fd\u7ef4\u62a4\u4e2a\u4eba\u8d44\u6599\u3002",
  login: "\u53bb\u767b\u5f55",
  current: "\u5f53\u524d\u8d26\u53f7",
  verified: "\u90ae\u7bb1\u5df2\u9a8c\u8bc1",
  unverified: "\u90ae\u7bb1\u672a\u9a8c\u8bc1",
  name: "\u6635\u79f0",
  school: "\u5b66\u6821",
  major: "\u4e13\u4e1a",
  grade: "\u5e74\u7ea7",
  chooseSchool: "\u9009\u62e9\u5b66\u6821",
  chooseMajor: "\u9009\u62e9\u4e13\u4e1a",
  clearMajor: "\u6682\u4e0d\u7ed1\u5b9a\u4e13\u4e1a",
  save: "\u4fdd\u5b58\u4e2a\u4eba\u8d44\u6599",
  saving: "\u4fdd\u5b58\u4e2d...",
  saved: "\u4e2a\u4eba\u8d44\u6599\u5df2\u66f4\u65b0\u3002",
  downloads: "\u67e5\u770b\u6211\u7684\u4e0b\u8f7d",
  forum: "\u6211\u7684\u8ba8\u8bba",
  notifications: "\u6211\u7684\u901a\u77e5",
  entitlements: "\u6211\u7684\u8d44\u6599\u6743\u9650",
  unlockedMaterials: "\u53ef\u8bbf\u95ee\u8d44\u6599",
  packageGrants: "\u8bfe\u7a0b\u5305",
  directMaterialGrants: "\u5355\u8d44\u6599",
  noEntitlements: "\u6682\u65e0\u5df2\u89e3\u9501\u7684\u4ed8\u8d39\u8d44\u6599\u6216\u8bfe\u7a0b\u5305\u3002",
  packageMaterials: "\u4efd\u8d44\u6599",
  expiresAt: "\u5230\u671f",
  neverExpires: "\u957f\u671f\u6709\u6548",
  entitlementsFailed: "\u6743\u9650\u6458\u8981\u6682\u65f6\u4e0d\u53ef\u7528",
  logout: "\u9000\u51fa\u767b\u5f55",
  loggedOut: "\u5df2\u9000\u51fa\u767b\u5f55\u3002",
  loadFailed: "\u4e2a\u4eba\u8d44\u6599\u6682\u65f6\u4e0d\u53ef\u7528",
  saveFailed: "\u4fdd\u5b58\u5931\u8d25",
};

export function ProfileForm() {
  const [user, setUser] = useState<User | null>(null);
  const [entitlements, setEntitlements] = useState<Entitlements | null>(null);
  const [schools, setSchools] = useState<School[]>([]);
  const [majors, setMajors] = useState<Major[]>([]);
  const [form, setForm] = useState<ProfileFormState>({ name: "", schoolId: "", majorId: "", grade: "2023" });
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const [entitlementError, setEntitlementError] = useState("");

  useEffect(() => {
    void loadProfile();
  }, []);

  const filteredMajors = useMemo(() => {
    return majors.filter((major) => !form.schoolId || major.schoolId === form.schoolId);
  }, [form.schoolId, majors]);

  async function loadProfile() {
    setLoading(true);
    setError("");
    setMessage("");
    try {
      const [meResponse, schoolsResponse, majorsResponse] = await Promise.all([
        request<User>("/auth/me", { method: "GET" }),
        request<{ schools: School[] }>("/schools", { method: "GET" }),
        request<{ majors: Major[] }>("/majors", { method: "GET" }),
      ]);
      const nextUser = meResponse.data ?? null;
      setUser(nextUser);
      setSchools(schoolsResponse.data?.schools ?? []);
      setMajors(majorsResponse.data?.majors ?? []);
      if (nextUser) {
        setForm({
          name: nextUser.name ?? "",
          schoolId: nextUser.schoolId ?? "",
          majorId: nextUser.majorId ?? "",
          grade: nextUser.grade || "2023",
        });
        await loadEntitlements();
      }
    } catch (err) {
      setUser(null);
      setEntitlements(null);
      setError(err instanceof Error ? err.message : copy.loadFailed);
    } finally {
      setLoading(false);
    }
  }

  async function loadEntitlements() {
    setEntitlementError("");
    try {
      const response = await request<Entitlements>("/me/entitlements", { method: "GET" });
      setEntitlements(response.data ?? null);
    } catch {
      setEntitlements(null);
      setEntitlementError(copy.entitlementsFailed);
    }
  }

  async function saveProfile(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSaving(true);
    setError("");
    setMessage("");
    try {
      const response = await request<User>("/auth/me", {
        method: "PATCH",
        body: JSON.stringify(form),
      });
      if (response.data) {
        setUser(response.data);
        setForm({
          name: response.data.name ?? "",
          schoolId: response.data.schoolId ?? "",
          majorId: response.data.majorId ?? "",
          grade: response.data.grade || "2023",
        });
      }
      setMessage(copy.saved);
    } catch (err) {
      setError(err instanceof Error ? err.message : copy.saveFailed);
    } finally {
      setSaving(false);
    }
  }

  async function logout() {
    setSaving(true);
    setError("");
    setMessage("");
    try {
      await request<{ ok: boolean }>("/auth/logout", { method: "POST" });
      setUser(null);
      setEntitlements(null);
      setMessage(copy.loggedOut);
    } catch (err) {
      setError(err instanceof Error ? err.message : copy.saveFailed);
    } finally {
      setSaving(false);
    }
  }

  function updateForm(patch: Partial<ProfileFormState>) {
    setForm((current) => ({ ...current, ...patch }));
  }

  function formatExpiry(value?: string) {
    if (!value) return copy.neverExpires;
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return `${copy.expiresAt}: ${value}`;
    return `${copy.expiresAt}: ${date.toLocaleDateString("zh-CN")}`;
  }

  if (loading) {
    return <p className="rounded-2xl border border-border bg-card p-4 text-sm text-muted-foreground">{copy.loading}</p>;
  }

  if (!user) {
    return (
      <div className="rounded-3xl border border-border bg-card p-5 shadow-sm">
        <p className="text-sm leading-6 text-muted-foreground">{copy.loginRequired}</p>
        <Link className="mt-4 inline-flex rounded-xl bg-primary px-4 py-2.5 text-sm font-medium text-primary-foreground" href="/login">
          {copy.login}
        </Link>
        {error ? <p className="mt-3 rounded-xl border border-red-200 bg-red-50 p-3 text-sm text-red-700">{error}</p> : null}
        {message ? <p className="mt-3 rounded-xl border border-border bg-muted p-3 text-sm text-foreground">{message}</p> : null}
      </div>
    );
  }

  return (
    <div className="grid gap-4 lg:grid-cols-[1fr_1.5fr]">
      <aside className="rounded-3xl border border-border bg-card p-5 shadow-sm">
        <p className="text-sm font-medium text-primary">{copy.current}</p>
        <h2 className="mt-2 break-words text-xl font-semibold">{user.email}</h2>
        <p className="mt-2 text-sm text-muted-foreground">
          {user.role} / {user.emailVerified ? copy.verified : copy.unverified}
        </p>
        <div className="mt-5 grid gap-2 text-sm text-muted-foreground">
          <Link className="rounded-xl border border-border px-3 py-2 text-foreground hover:bg-muted" href="/me/downloads">
            {copy.downloads}
          </Link>
          <Link className="rounded-xl border border-border px-3 py-2 text-foreground hover:bg-muted" href="/me/forum">
            {copy.forum}
          </Link>
          <Link className="rounded-xl border border-border px-3 py-2 text-foreground hover:bg-muted" href="/me/notifications">
            {copy.notifications}
          </Link>
          <button
            className="rounded-xl border border-border px-3 py-2 text-left text-foreground hover:bg-muted disabled:cursor-not-allowed disabled:opacity-60"
            disabled={saving}
            onClick={logout}
            type="button"
          >
            {copy.logout}
          </button>
        </div>
        <div className="mt-6 rounded-2xl border border-border bg-background p-4">
          <p className="text-sm font-medium text-foreground">{copy.entitlements}</p>
          {entitlements ? (
            <>
              <div className="mt-3 grid grid-cols-3 gap-2 text-center text-xs text-muted-foreground">
                <div className="rounded-xl border border-border bg-card p-2">
                  <strong className="block text-base text-foreground">{entitlements.summary.unlockedMaterials}</strong>
                  {copy.unlockedMaterials}
                </div>
                <div className="rounded-xl border border-border bg-card p-2">
                  <strong className="block text-base text-foreground">{entitlements.summary.packageGrants}</strong>
                  {copy.packageGrants}
                </div>
                <div className="rounded-xl border border-border bg-card p-2">
                  <strong className="block text-base text-foreground">{entitlements.summary.directMaterialGrants}</strong>
                  {copy.directMaterialGrants}
                </div>
              </div>
              {entitlements.packageGrants.length > 0 || entitlements.materialGrants.length > 0 ? (
                <div className="mt-3 space-y-2">
                  {entitlements.packageGrants.slice(0, 3).map((row) => (
                    <div className="rounded-xl border border-border bg-card p-3 text-sm" key={row.grant.id}>
                      <p className="font-medium text-foreground">{row.package?.title ?? row.grant.packageId}</p>
                      <p className="mt-1 text-xs text-muted-foreground">
                        {row.materials.length} {copy.packageMaterials} / {formatExpiry(row.grant.expiresAt)}
                      </p>
                    </div>
                  ))}
                  {entitlements.materialGrants.slice(0, 3).map((row) => (
                    <div className="rounded-xl border border-border bg-card p-3 text-sm" key={row.grant.id}>
                      <p className="font-medium text-foreground">{row.material?.title ?? row.grant.materialId}</p>
                      <p className="mt-1 text-xs text-muted-foreground">{formatExpiry(row.grant.expiresAt)}</p>
                    </div>
                  ))}
                </div>
              ) : (
                <p className="mt-3 text-sm leading-6 text-muted-foreground">{copy.noEntitlements}</p>
              )}
            </>
          ) : (
            <p className="mt-3 text-sm leading-6 text-muted-foreground">{entitlementError || copy.noEntitlements}</p>
          )}
        </div>
      </aside>

      <form className="rounded-3xl border border-border bg-card p-5 shadow-sm" onSubmit={saveProfile}>
        <div className="grid gap-4 sm:grid-cols-2">
          <label className="block text-sm font-medium text-foreground">
            {copy.name}
            <input
              className="mt-2 w-full rounded-xl border border-border bg-background px-3 py-2 text-sm shadow-sm"
              onChange={(event) => updateForm({ name: event.target.value })}
              value={form.name}
            />
          </label>
          <label className="block text-sm font-medium text-foreground">
            {copy.grade}
            <input
              className="mt-2 w-full rounded-xl border border-border bg-background px-3 py-2 text-sm shadow-sm"
              onChange={(event) => updateForm({ grade: event.target.value })}
              placeholder="2023\u7ea7"
              value={form.grade}
            />
          </label>
          <label className="block text-sm font-medium text-foreground">
            {copy.school}
            <select
              className="mt-2 w-full rounded-xl border border-border bg-background px-3 py-2 text-sm shadow-sm"
              onChange={(event) => updateForm({ schoolId: event.target.value, majorId: "" })}
              value={form.schoolId}
            >
              <option value="">{copy.chooseSchool}</option>
              {schools.map((school) => (
                <option key={school.id} value={school.id}>
                  {school.name}
                </option>
              ))}
            </select>
          </label>
          <label className="block text-sm font-medium text-foreground">
            {copy.major}
            <select
              className="mt-2 w-full rounded-xl border border-border bg-background px-3 py-2 text-sm shadow-sm"
              onChange={(event) => updateForm({ majorId: event.target.value })}
              value={form.majorId}
            >
              <option value="">{copy.clearMajor}</option>
              {filteredMajors.map((major) => (
                <option key={major.id} value={major.id}>
                  {major.name}
                </option>
              ))}
            </select>
          </label>
        </div>

        <button
          className="mt-5 w-full rounded-xl bg-primary px-4 py-2.5 text-sm font-medium text-primary-foreground shadow-sm transition hover:bg-[#254d42] disabled:cursor-not-allowed disabled:opacity-60 sm:w-auto"
          disabled={saving}
          type="submit"
        >
          {saving ? copy.saving : copy.save}
        </button>

        {message ? <p className="mt-3 rounded-xl border border-border bg-muted p-3 text-sm text-foreground">{message}</p> : null}
        {error ? <p className="mt-3 rounded-xl border border-red-200 bg-red-50 p-3 text-sm text-red-700">{error}</p> : null}
      </form>
    </div>
  );
}

async function request<T>(path: string, init: RequestInit): Promise<Envelope<T>> {
  const headers = new Headers(init.headers);
  if (!(init.body instanceof FormData)) {
    headers.set("Content-Type", "application/json");
  }
  const response = await fetch(`${apiBaseUrl()}${path}`, {
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
