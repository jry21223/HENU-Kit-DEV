"use client";

import Link from "next/link";
import { useCallback, useEffect, useRef, useState } from "react";
import { useAccountConsoleUnauthorizedHandler } from "@/components/account/account-console-session";
import { useReveal } from "@/components/account/use-reveal";
import {
  fetchAccountNotifications,
  formatPortalError,
  markAccountNotificationRead,
} from "@/lib/api/client";
import type { AccountNotification, AccountNotificationsResponse } from "@/lib/api/types";
import { formatTimestamp } from "@/lib/format-date";
import { commandKey } from "@/lib/idempotency";

type NotificationsState =
  | { kind: "loading" }
  | { kind: "success"; response: AccountNotificationsResponse }
  | { kind: "error"; message: string };

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
  const [markingAll, setMarkingAll] = useState(false);
  const writePending = useRef(false);
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
      return true;
    } catch (error) {
      if (!handleUnauthorized(error)) setCommandError(formatPortalError(error));
      return false;
    } finally {
      setPendingID(null);
    }
  };

  const notifications = state.kind === "success" ? state.response.data.notifications : [];
  const unreadNotifications = notifications.filter((notification) => !notification.read_at);
  const markMany = async (items: AccountNotification[], all: boolean) => {
    if (writePending.current) return;
    writePending.current = true;
    setMarkingAll(all);
    try {
      for (const notification of items) {
        if (!await markRead(notification.id)) break;
      }
    } finally {
      writePending.current = false;
      setMarkingAll(false);
    }
  };
  return (
    <div>
      <section data-enter className="border-b border-ink pb-5">
        <p className="font-mono text-xs tracking-[0.3em] text-ink/60">
          <span className="text-accent-text">A-06</span>
          <span className="mx-2">/</span>
          NOTIFICATIONS
        </p>
        <h1 className="mt-3 font-display text-4xl font-bold tracking-tight">系统通知</h1>
        {unreadNotifications.length >= 2 || markingAll ? (
          <button type="button" disabled={markingAll || pendingID !== null} onClick={() => void markMany(unreadNotifications, true)} className="mt-3 min-h-11 border border-ink px-3 text-sm focus-visible:outline-2 focus-visible:outline-focus-ring disabled:opacity-50">
            {markingAll ? "正在标为已读…" : "全部标为已读"}
          </button>
        ) : null}
      </section>

      {state.kind === "loading" ? (
        <section
          data-account-notifications-state="loading"
          aria-live="polite"
          className="mt-6 border border-line px-5 py-8 font-mono text-xs text-ink/60"
        >
          通知加载中<span aria-hidden className="animate-pulse text-accent-text">…</span>
        </section>
      ) : null}

      {state.kind === "error" ? (
        <section data-account-notifications-state="error" role="alert" className="mt-6 border border-accent px-5 py-6">
          <p className="font-mono text-xs text-accent-text">通知暂时不可用</p>
          <p className="mt-3 text-sm leading-6 text-ink/65">{state.message}</p>
          <button
            type="button"
            onClick={() => {
              setState({ kind: "loading" });
              loadNotifications();
            }}
            className="mt-5 inline-flex min-h-11 items-center justify-center border border-ink px-4 py-2 font-mono text-xs transition-colors hover:bg-ink hover:text-paper"
          >
            重新加载
          </button>
        </section>
      ) : null}

      {state.kind === "success" ? (
        <section data-account-notifications-state="success" className="mt-6">
          {commandError ? (
            <p role="alert" className="mb-4 border border-accent px-4 py-3 text-sm leading-6 text-accent-text">
              {commandError}
            </p>
          ) : null}
          {notifications.length === 0 ? (
            <div data-account-notifications-empty className="border-y border-line py-8">
              <p className="font-display text-xl font-bold">暂无系统通知</p>
              <p className="mt-2 text-sm leading-6 text-ink/60">客服回复、工单状态变化等消息会显示在这里。</p>
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
                            <span className="border border-accent px-2 py-0.5 font-mono text-xs text-accent-text">未读</span>
                          ) : (
                            <span className="font-mono text-xs text-ink/60">已读</span>
                          )}
                        </div>
                        {notification.ticket_reference ? (
                          <p className="mt-2 font-mono text-xs text-ink/60">
                            <Link
                              href={`/account/tickets?ticket=${encodeURIComponent(notification.ticket_reference)}`}
                              className="inline-flex min-h-11 items-center underline underline-offset-4 transition-colors hover:text-accent-text"
                            >
                              关联工单 <span className="ml-1 tracking-[0.12em]">{notification.ticket_reference}</span> →
                            </Link>
                          </p>
                        ) : null}
                        <p className="mt-3 whitespace-pre-wrap text-sm leading-6 text-ink/75">{notification.body}</p>
                        <p className="mt-3 font-mono text-xs text-ink/60">{formatTimestamp(notification.created_at)}</p>
                      </div>
                      {unread ? (
                        <button
                          type="button"
                          onClick={() => void markMany([notification], false)}
                          disabled={markingAll || pendingID !== null}
                          className="inline-flex min-h-11 shrink-0 items-center justify-center border border-ink px-3 py-2 font-mono text-xs transition-colors hover:bg-ink hover:text-paper disabled:cursor-wait disabled:opacity-50"
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
