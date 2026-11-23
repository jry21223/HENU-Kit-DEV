"use client";

import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { formatPortalError, PortalUnauthorizedError } from "@/lib/api/client";
import type { CampusItem, CampusMessage } from "@/lib/api/types";
import { completeCampusItem, CONTACT_CHANNELS, postCampusMessage, readCampusContactBook, readCampusExchanges, requestCampusExchange, respondCampusExchange, type ContactBook, type ContactChannel, type Exchange } from "@/lib/campus/gateway";
import {
  CampusContactDialog,
  formatCampusUnavailableMessage,
  isCampusWritesUnavailable,
  loginForCampus,
  useCampusViewer,
} from "./campus-actions";

const CHANNELS = Object.keys(CONTACT_CHANNELS) as ContactChannel[];
const EXCHANGE_LABELS = { requested: "待回复", accepted: "已同意", declined: "已婉拒", cancelled: "已取消" };

export function CampusMessageForm({ id, onMessage }: { id: string; onMessage: (message: CampusMessage) => void }) {
  const { ready, user } = useCampusViewer();
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(false);
  const [failure, setFailure] = useState("");
  const [expired, setExpired] = useState(false);
  const path = `/campus/item/${encodeURIComponent(id)}`;
  if (!ready) return null;
  if (!user || expired) return <Button asChild variant="outline"><Link href={loginForCampus(path)}>登录后留言</Link></Button>;
  return (
    <form className="space-y-2" onSubmit={async (event) => {
      event.preventDefault();
      if (busy || !text.trim()) return;
      setBusy(true);
      setFailure("");
      try {
        const { message } = await postCampusMessage(id, text.trim());
        onMessage(message);
        setText("");
      } catch (error) {
        setExpired(error instanceof PortalUnauthorizedError);
        setFailure(formatPortalError(error));
      } finally {
        setBusy(false);
      }
    }}>
      <label className="block text-sm">写留言<Textarea className="mt-2" value={text} onChange={(event) => setText(event.target.value)} maxLength={500} required disabled={busy} /></label>
      <p className="text-xs text-ink/70">留言公开可见，请勿填写私人联系方式。</p>
      <Button type="submit" disabled={busy || !text.trim()}>{busy ? "正在发送…" : "发送留言"}</Button>
      {failure && <p role="alert" className="text-sm text-accent-text">{failure}</p>}
    </form>
  );
}

export function CampusExchangePanel({ item, onStatus }: { item: CampusItem; onStatus: (status: CampusItem["status"]) => void }) {
  const { user, ready } = useCampusViewer();
  const path = `/campus/item/${encodeURIComponent(item.id)}`;
  if (!ready) return null;
  return (
    <section className="border border-line p-5">
      <h2 className="text-lg font-bold">交换联系方式</h2>
      <p className="mt-2 text-sm text-ink/70">双方同意后，对方才能看到你选择的联系方式和备注。</p>
      {user ? <ExchangeActions key={`${user.uid}:${item.id}`} item={item} viewerID={user.uid} path={path} onStatus={onStatus} /> : (
        <Button asChild variant="outline" className="mt-3"><Link href={loginForCampus(path)}>登录后交换</Link></Button>
      )}
    </section>
  );
}

type ExchangeView = Awaited<ReturnType<typeof readCampusExchanges>>;

function ExchangeActions({ item, viewerID, path, onStatus }: { item: CampusItem; viewerID: string; path: string; onStatus: (status: CampusItem["status"]) => void }) {
  const [book, setBook] = useState<ContactBook>();
  const [view, setView] = useState<ExchangeView>();
  const [selected, setSelected] = useState<ContactChannel[]>([]);
  const [busy, setBusy] = useState(false);
  const [failure, setFailure] = useState("");
  const [expired, setExpired] = useState(false);
  const [unavailable, setUnavailable] = useState(false);
  const [attempt, setAttempt] = useState(0);
  const inFlight = useRef(false);
  const itemID = item.id;
  useEffect(() => {
    const controller = new AbortController();
    void Promise.all([readCampusContactBook(controller.signal), readCampusExchanges(itemID, controller.signal)]).then(([contact, exchanges]) => {
      if (controller.signal.aborted) return;
      if (exchanges.viewer_user_id !== viewerID) {
        setExpired(true);
        return;
      }
      setUnavailable(false);
      setBook(contact.contact);
      setView(exchanges);
    }).catch((error: unknown) => {
      if (controller.signal.aborted) return;
      const serviceUnavailable = isCampusWritesUnavailable(error);
      setUnavailable(serviceUnavailable);
      setExpired(error instanceof PortalUnauthorizedError);
      setFailure(serviceUnavailable ? formatCampusUnavailableMessage(error) : formatPortalError(error));
    });
    return () => controller.abort();
  }, [itemID, viewerID, attempt]);

  const channels = selected.filter((channel) => Boolean(book?.[channel]?.trim()));
  async function run(action: () => Promise<void>) {
    if (inFlight.current) return;
    inFlight.current = true;
    setBusy(true);
    setFailure("");
    try {
      await action();
    } catch (error) {
      setExpired(error instanceof PortalUnauthorizedError);
      setFailure(formatPortalError(error));
    } finally {
      inFlight.current = false;
      setBusy(false);
    }
  }

  function updateExchange(exchange: Exchange) {
    setView((current) => current && { ...current, exchanges: [...current.exchanges.filter((entry) => entry.id !== exchange.id), exchange] });
    setSelected([]);
  }

  if (expired) return <Button asChild className="mt-3" variant="outline"><Link href={loginForCampus(path)}>重新登录</Link></Button>;
  if (!view || !book) {
    if (unavailable) {
      return (
        <div role="status" className="mt-3 space-y-1 border border-ink/20 bg-paper-raised p-3">
          <p className="font-mono text-xs text-ink/70">暂未开启</p>
          <p className="text-sm leading-6 text-ink">{failure}</p>
        </div>
      );
    }
    return failure ? (
      <div className="mt-3 space-y-2"><p role="alert" className="text-sm text-accent-text">{failure}</p><Button variant="outline" onClick={() => { setFailure(""); setAttempt((value) => value + 1); }}>重新加载</Button></div>
    ) : <p role="status" className="mt-3 text-sm">正在加载交换申请…</p>;
  }

  const eligible = view.exchanges.filter((exchange) => exchange.requester_user_id === viewerID || exchange.owner_user_id === viewerID);
  const canRequest = !view.is_owner && item.status === "open" && !eligible.some((exchange) => exchange.status === "requested" || exchange.status === "accepted");
  const canAccept = view.is_owner && item.status === "open" && eligible.some((exchange) => exchange.status === "requested");
  return (
    <div className="mt-3 space-y-4">
      {(canRequest || canAccept) && (
        <fieldset disabled={busy}>
          <legend className="text-sm font-medium">选择你愿意分享的渠道</legend>
          {CHANNELS.filter((channel) => Boolean(book[channel]?.trim())).map((channel) => (
            <label className="flex min-h-11 items-center gap-2 text-sm" key={channel}>
              <input type="checkbox" checked={selected.includes(channel)} className="size-4 accent-ink focus-visible:outline-2 focus-visible:outline-focus-ring" onChange={(event) => setSelected(event.target.checked ? [...selected, channel] : selected.filter((value) => value !== channel))} />
              {CONTACT_CHANNELS[channel]}
            </label>
          ))}
          {book.note && <p className="break-words text-xs text-ink/70">备注也会分享给对方。{book.note}</p>}
          <div className="mt-2"><CampusContactDialog path={path} onSaved={setBook} /></div>
        </fieldset>
      )}
      {canRequest && <Button disabled={busy || channels.length === 0} onClick={() => void run(async () => updateExchange((await requestCampusExchange(itemID, channels)).exchange))}>申请交换</Button>}
      {eligible.map((exchange) => (
        <div key={exchange.id} className="space-y-2 border-t border-line pt-3">
          <p className="break-words text-sm">{view.is_owner ? exchange.requester_name || "申请人" : "我的申请"} · {EXCHANGE_LABELS[exchange.status]}</p>
          {exchange.status === "requested" && <p className="text-xs text-ink/70">申请共享渠道：{exchange.requester_channels.map((channel) => CONTACT_CHANNELS[channel]).join("、")}</p>}
          {view.is_owner && exchange.status === "requested" && item.status === "open" && (
            <div className="flex flex-wrap gap-2">
              <Button disabled={busy || channels.length === 0} onClick={() => void run(async () => {
                const response = await respondCampusExchange(itemID, exchange.id, "accept", channels);
                updateExchange(response.exchange);
                onStatus(response.item_status);
              })}>同意交换</Button>
              <Button variant="outline" disabled={busy} onClick={() => void run(async () => updateExchange((await respondCampusExchange(itemID, exchange.id, "decline")).exchange))}>婉拒</Button>
            </div>
          )}
          {exchange.status === "accepted" && <SharedContact contact={exchange.owner_user_id === viewerID ? exchange.requester_snapshot : exchange.owner_snapshot} />}
        </div>
      ))}
      {view.is_owner && (item.status === "open" || item.status === "ongoing") && <Button variant="outline" disabled={busy} onClick={() => void run(async () => onStatus((await completeCampusItem(itemID)).status))}>标记已完成</Button>}
      {!canRequest && eligible.length === 0 && <p className="text-sm text-ink/70">暂无交换申请。</p>}
      {busy && <p role="status" className="text-sm">正在提交…</p>}
      {failure && <p role="alert" className="text-sm text-accent-text">{failure}</p>}
    </div>
  );
}

function SharedContact({ contact }: { contact?: Partial<ContactBook> }) {
  const [announcement, setAnnouncement] = useState("");
  return (
    <div className="space-y-2 border border-line p-3">
      <h3 className="text-sm font-bold">已交换联系方式</h3>
      {CHANNELS.map((channel) => {
        const value = contact?.[channel];
        return value ? (
          <div className="flex flex-wrap items-center justify-between gap-2" key={channel}>
            <p className="min-w-0 break-all text-sm">{CONTACT_CHANNELS[channel]} · {value}</p>
            <Button variant="outline" size="sm" onClick={async () => {
              try {
                await navigator.clipboard.writeText(value);
                setAnnouncement(`已复制${CONTACT_CHANNELS[channel]}。`);
              } catch {
                setAnnouncement("复制失败，请选中号码手动复制。");
              }
            }}>复制{CONTACT_CHANNELS[channel]}</Button>
          </div>
        ) : null;
      })}
      {contact?.note && <p className="break-words text-sm">备注 · {contact.note}</p>}
      <p role="status" className="text-xs">{announcement}</p>
    </div>
  );
}
