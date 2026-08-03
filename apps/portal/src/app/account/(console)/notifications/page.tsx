"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useAccountConsoleUnauthorizedHandler } from "@/components/account/account-console-session";
import { useReveal } from "@/components/account/use-reveal";
import {
  fetchAccountNotifications,
  formatPortalError,
  markAccountNotificationRead,
} from "@/lib/api/client";
import type { AccountNotification, AccountNotificationsResponse } from "@/lib/api/types";

type NotificationsState =
  | { kind: "loading" }
  | { kind: "success"; response: AccountNotificationsResponse }
  | { kind: "error"; message: string };

function commandKey(prefix: string): string {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return `${prefix}:${crypto.randomUUID()}`;
  }
  return `${prefix}:${Date.now().toString(36)}:${Math.random().toString(36).slice(2)}`;
}

function formatTimestamp(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat("zh-CN", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(date);
}

function replaceNotification(
  state: NotificationsState,
  notification: AccountNotification
): NotificationsState {
  if (state.kind !== "success") return state;
  return {
    kind: "success",
    response: {
      ...state.response,
      data: {
        notifications: state.response.data.notifications.map((current) =>
          current.id === notification.id ? notification : current
        ),
      },
    },
  };
}

export default function NotificationsPage() {
  const [state, setState] = useState<NotificationsState>({ kind: "loading" });
  const [pendingID, setPendingID] = useState<string | null>(null);
  const [commandError, setCommandError] = useState("");
  const requestVersion = useRef(0);
  const readKeys = useRef(new Map<string, string>());
  const handleUnauthorized = useAccountConsoleUnauthorizedHandler();
  useReveal();

  const loadNotifications = useCallback(() => {
    const version = ++requestVersion.current;
    void fetchAccountNotifications().then(
      (response) => {
        if (version === requestVersion.current) setState({ kind: "success", response });
      },
      (error: unknown) => {
        if (version === requestVersion.current && !handleUnauthorized(error)) {
          setState({ kind: "error", message: formatPortalError(error) });
        }
      }
    );
  }, [handleUnauthorized]);

  useEffect(() => {
    loadNotifications();
    return () => {
      requestVersion.current += 1;
    };
  }, [loadNotifications]);

  const markRead = async (notificationID: string) => {
    const idempotencyKey = readKeys.current.get(notificationID) ?? commandKey("portal-notification");
    readKeys.current.set(notificationID, idempotencyKey);
    setPendingID(notificationID);
    setCommandError("");
    try {
      const response = await markAccountNotificationRead(notificationID, idempotencyKey);
      readKeys.current.delete(notificationID);
      setState((current) => replaceNotification(current, response.data.notification));
    } catch (error) {
      if (!handleUnauthorized(error)) setCommandError(formatPortalError(error));
    } finally {
      setPendingID(null);
    }
  };

  const notifications = state.kind === "success" ? state.response.data.notifications : [];
  return (
    <div>
      <section data-enter className="border-b border-ink pb-5">
        <p className="font-mono text-xs tracking-[0.3em] text-ink/55">
          <span className="text-accent">A-06</span>
          <span className="mx-2">/</span>
          NOTIFICATIONS
        </p>
        <h1 className="mt-3 font-display text-4xl font-bold tracking-tight">系统通知</h1>
        <p className="mt-3 max-w-2xl text-sm leading-6 text-ink/60">
          通知记录由系统保存；标记已读只会更新对应的通知记录。
        </p>
      </section>

      {state.kind === "loading" ? (
        <section
          data-account-notifications-state="loading"
          aria-live="polite"
          className="mt-6 border border-line px-5 py-8 font-mono text-xs tracking-[0.2em] text-ink/50"
        >
          NOTIFICATIONS LOADING<span className="animate-pulse text-accent">…</span>
        </section>
      ) : null}

      {state.kind === "error" ? (
        <section data-account-notifications-state="error" role="alert" className="mt-6 border border-accent px-5 py-6">
          <p className="font-mono text-xs tracking-[0.14em] text-accent">NOTIFICATIONS UNAVAILABLE</p>
          <p className="mt-3 text-sm leading-6 text-ink/65">{state.message}</p>
          <button
            type="button"
            onClick={() => {
              setState({ kind: "loading" });
              loadNotifications();
            }}
            className="mt-5 inline-flex min-h-11 items-center justify-center border border-ink px-4 py-2 font-mono text-xs tracking-widest transition-colors hover:bg-ink hover:text-paper"
          >
            重新加载
          </button>
        </section>
      ) : null}

      {state.kind === "success" ? (
        <section data-account-notifications-state="success" className="mt-6">
          {commandError ? (
            <p role="alert" className="mb-4 border border-accent px-4 py-3 text-sm leading-6 text-accent">
              {commandError}
            </p>
          ) : null}
          {notifications.length === 0 ? (
            <div data-account-notifications-empty className="border-y border-line py-8">
              <p className="font-display text-xl font-bold">暂无系统通知</p>
              <p className="mt-2 text-sm leading-6 text-ink/60">客服回复、工单状态变化等消息会在这里持久化展示。</p>
            </div>
          ) : (
            <div className="border-t border-ink">
              {notifications.map((notification) => {
                const unread = !notification.read_at;
                return (
                  <article key={notification.id} className="border-b border-line py-5">
                    <div className="flex flex-wrap items-start justify-between gap-4">
                      <div className="min-w-0">
                        <div className="flex flex-wrap items-center gap-2">
                          <h2 className="font-display text-xl font-bold">{notification.title}</h2>
                          {unread ? (
                            <span className="border border-accent px-2 py-0.5 font-mono text-[10px] tracking-widest text-accent">未读</span>
                          ) : (
                            <span className="font-mono text-[10px] tracking-widest text-ink/40">已读</span>
                          )}
                        </div>
                        {notification.ticket_reference ? (
                          <p className="mt-2 font-mono text-[10px] tracking-[0.12em] text-ink/45">{notification.ticket_reference}</p>
                        ) : null}
                        <p className="mt-3 whitespace-pre-wrap text-sm leading-6 text-ink/75">{notification.body}</p>
                        <p className="mt-3 font-mono text-[10px] text-ink/45">{formatTimestamp(notification.created_at)}</p>
                      </div>
                      {unread ? (
                        <button
                          type="button"
                          onClick={() => void markRead(notification.id)}
                          disabled={pendingID === notification.id}
                          className="inline-flex min-h-11 shrink-0 items-center justify-center border border-ink px-3 py-2 font-mono text-xs tracking-widest transition-colors hover:bg-ink hover:text-paper disabled:cursor-wait disabled:opacity-50"
                        >
                          {pendingID === notification.id ? "正在更新…" : "标为已读"}
                        </button>
                      ) : null}
                    </div>
                  </article>
                );
              })}
            </div>
          )}
        </section>
      ) : null}
    </div>
  );
}
