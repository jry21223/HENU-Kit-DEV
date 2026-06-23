"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { Bell, CheckCircle2, MailOpen } from "lucide-react";
import { SiteShell } from "@/components/layout/site-shell";
import { Badge } from "@/components/ui/badge";
import { apiBaseUrl, type NotificationItem } from "@/lib/api";

type Envelope<T> = {
  code: number;
  message: string;
  data?: T;
};

const copy = {
  back: "\u8fd4\u56de\u4e2a\u4eba\u4e2d\u5fc3",
  eyebrow: "\u6211\u7684\u901a\u77e5",
  title: "\u5ba1\u6838\u7ed3\u679c\u4e0e\u7cfb\u7edf\u6d88\u606f",
  intro:
    "\u8fd9\u91cc\u53ea\u5c55\u793a\u5f53\u524d\u767b\u5f55\u8d26\u53f7\u7684\u901a\u77e5\u3002\u8bba\u575b\u5e16\u5b50\u548c\u56de\u590d\u7684\u5ba1\u6838\u7ed3\u679c\u4f1a\u5728\u670d\u52a1\u7aef\u4e8b\u52a1\u4e2d\u5199\u5165\u901a\u77e5\u3002",
  loading: "\u6b63\u5728\u52a0\u8f7d\u901a\u77e5...",
  login: "\u53bb\u767b\u5f55",
  unread: "\u672a\u8bfb",
  read: "\u5df2\u8bfb",
  markRead: "\u6807\u8bb0\u5df2\u8bfb",
  readAll: "\u5168\u90e8\u6807\u8bb0\u5df2\u8bfb",
  empty: "\u6682\u65e0\u901a\u77e5\u3002",
  fallbackError: "\u901a\u77e5\u6682\u65f6\u4e0d\u53ef\u7528",
};

export default function NotificationsPage() {
  const [notifications, setNotifications] = useState<NotificationItem[]>([]);
  const [unreadCount, setUnreadCount] = useState(0);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  async function loadNotifications() {
    setLoading(true);
    setError("");
    try {
      const response = await request<{ notifications: NotificationItem[]; unreadCount: number }>("/me/notifications");
      setNotifications(response.data?.notifications ?? []);
      setUnreadCount(response.data?.unreadCount ?? 0);
    } catch (err) {
      setError(err instanceof Error ? err.message : copy.fallbackError);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void loadNotifications();
  }, []);

  async function markRead(id: string) {
    setSaving(true);
    setError("");
    try {
      await request<{ read: boolean; notificationId: string }>(`/me/notifications/${id}/read`, { method: "POST" });
      await loadNotifications();
    } catch (err) {
      setError(err instanceof Error ? err.message : copy.fallbackError);
    } finally {
      setSaving(false);
    }
  }

  async function markAllRead() {
    setSaving(true);
    setError("");
    try {
      await request<{ read: boolean; count: number }>("/me/notifications/read-all", { method: "POST" });
      await loadNotifications();
    } catch (err) {
      setError(err instanceof Error ? err.message : copy.fallbackError);
    } finally {
      setSaving(false);
    }
  }

  return (
    <SiteShell>
      <nav className="flex flex-wrap items-center justify-between gap-3 text-sm">
        <Link className="font-semibold text-primary" href="/me">
          {copy.back}
        </Link>
        <button
          className="rounded-xl border border-border px-3 py-2 font-medium text-foreground hover:bg-muted disabled:cursor-not-allowed disabled:opacity-60"
          disabled={saving || unreadCount === 0}
          onClick={markAllRead}
          type="button"
        >
          {copy.readAll}
        </button>
      </nav>

      <section className="rounded-3xl border border-border bg-card p-5 shadow-sm sm:p-6">
        <div className="flex flex-col gap-5 sm:flex-row sm:items-end sm:justify-between">
          <div>
            <p className="text-sm font-medium text-primary">{copy.eyebrow}</p>
            <h1 className="mt-2 break-words text-3xl font-semibold tracking-tight sm:text-4xl">{copy.title}</h1>
            <p className="mt-3 max-w-3xl text-sm leading-6 text-muted-foreground">{copy.intro}</p>
          </div>
          <div className="rounded-2xl border border-border bg-background p-4 text-sm">
            <p className="text-xs text-muted-foreground">{copy.unread}</p>
            <p className="mt-1 flex items-center text-2xl font-semibold">
              <Bell className="mr-2 size-5 text-primary" aria-hidden="true" />
              {unreadCount}
            </p>
          </div>
        </div>
      </section>

      {loading ? <p className="rounded-2xl border border-border bg-card p-4 text-sm text-muted-foreground">{copy.loading}</p> : null}
      {error ? (
        <div className="rounded-2xl border border-border bg-card p-4 text-sm text-muted-foreground">
          <p>{error}</p>
          <Link className="mt-3 inline-flex rounded-xl bg-primary px-4 py-2.5 text-sm font-medium text-primary-foreground" href="/login">
            {copy.login}
          </Link>
        </div>
      ) : null}

      {!loading && !error ? (
        <section className="grid gap-3">
          {notifications.map((item) => {
            const unread = !item.readAt;
            return (
              <article className="rounded-3xl border border-border bg-card p-5 shadow-sm" key={item.id}>
                <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <Badge tone={unread ? "success" : "muted"}>{unread ? copy.unread : copy.read}</Badge>
                      <Badge tone="muted">{item.type}</Badge>
                    </div>
                    <h2 className="mt-3 break-words text-lg font-semibold">{item.title}</h2>
                    <p className="mt-2 whitespace-pre-wrap break-words text-sm leading-6 text-muted-foreground">{item.body}</p>
                  </div>
                  {unread ? (
                    <button
                      className="inline-flex shrink-0 items-center justify-center rounded-xl border border-border px-3 py-2 text-sm font-medium hover:bg-muted disabled:cursor-not-allowed disabled:opacity-60"
                      disabled={saving}
                      onClick={() => markRead(item.id)}
                      type="button"
                    >
                      <CheckCircle2 className="mr-2 size-4 text-primary" aria-hidden="true" />
                      {copy.markRead}
                    </button>
                  ) : (
                    <span className="inline-flex shrink-0 items-center rounded-xl border border-border px-3 py-2 text-sm text-muted-foreground">
                      <MailOpen className="mr-2 size-4" aria-hidden="true" />
                      {copy.read}
                    </span>
                  )}
                </div>
                <p className="mt-4 text-xs text-muted-foreground">{formatDate(item.createdAt)}</p>
              </article>
            );
          })}
          {notifications.length === 0 ? <p className="rounded-2xl border border-border bg-card p-4 text-sm text-muted-foreground">{copy.empty}</p> : null}
        </section>
      ) : null}
    </SiteShell>
  );
}

function formatDate(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString("zh-CN", { hour12: false });
}

async function request<T>(path: string, init: RequestInit = {}): Promise<Envelope<T>> {
  const headers = new Headers(init.headers);
  if (init.body && !(init.body instanceof FormData)) {
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
