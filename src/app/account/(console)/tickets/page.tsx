"use client";

import { useRef, useState } from "react";
import { useSyncExternalStore } from "react";
import { accountStore, Ticket } from "@/lib/auth/mock";
import { useReveal } from "@/components/account/use-reveal";
import { gsap, REDUCED_MOTION } from "@/lib/gsap";
import { cn } from "@/lib/cn";

const STATUS_CLS = {
  待处理: "border-accent text-accent",
  处理中: "border-ink text-ink",
  已解决: "border-line text-ink/40",
} as const;

function TicketRow({ ticket }: { ticket: Ticket }) {
  const [open, setOpen] = useState(false);
  const bodyRef = useRef<HTMLDivElement>(null);

  const toggle = () => {
    const el = bodyRef.current;
    const reduced = window.matchMedia(REDUCED_MOTION).matches;
    if (!open) {
      setOpen(true);
      if (el) {
        if (reduced) gsap.set(el, { height: "auto" });
        else gsap.to(el, { height: "auto", duration: 0.3, ease: "power2.out" });
      }
    } else {
      setOpen(false);
      if (el) {
        if (reduced) gsap.set(el, { height: 0 });
        else gsap.to(el, { height: 0, duration: 0.25, ease: "power2.in" });
      }
    }
  };

  return (
    <li className="border-b border-line">
      <button
        type="button"
        onClick={toggle}
        className="grid w-full grid-cols-[4rem_1fr_4.5rem] items-center gap-3 py-4 text-left transition-colors hover:bg-ink/[0.03] md:grid-cols-[5rem_1fr_6rem_5rem_4rem]"
      >
        <span className="font-mono text-xs text-ink/40">{ticket.id}</span>
        <span className="truncate text-sm">{ticket.title}</span>
        <span className={cn("w-fit border px-1.5 py-0.5 font-mono text-[10px]", STATUS_CLS[ticket.status])}>
          {ticket.status}
        </span>
        <span className="hidden font-mono text-[11px] text-ink/50 md:block">{ticket.type}</span>
        <span className="hidden text-right font-mono text-[11px] text-ink/50 md:block">
          {open ? "收起 −" : "展开 +"}
        </span>
      </button>
      <div ref={bodyRef} className="h-0 overflow-hidden">
        <div className="space-y-3 pb-5 pl-4 pr-2 md:pl-16">
          {ticket.msgs.map((msg, i) => (
            <div key={i} className="border-l-2 border-line pl-3">
              <p className="font-mono text-[10px] text-ink/50">
                <span className={msg.from === "客服" ? "text-accent" : ""}>{msg.from}</span>
                <span className="mx-2">·</span>
                {msg.time}
              </p>
              <p className="mt-1 text-sm leading-6 text-ink/80">{msg.text}</p>
            </div>
          ))}
        </div>
      </div>
    </li>
  );
}

export default function TicketsPage() {
  const data = useSyncExternalStore(accountStore.subscribe, accountStore.get, accountStore.getServer);
  useReveal();

  const [formOpen, setFormOpen] = useState(false);
  const [title, setTitle] = useState("");
  const [type, setType] = useState("功能异常");
  const [desc, setDesc] = useState("");
  const [error, setError] = useState("");

  const submit = () => {
    if (!title.trim()) return setError("请输入工单标题");
    if (!desc.trim()) return setError("请输入问题描述");
    accountStore.addTicket(title.trim(), type, desc.trim());
    setTitle("");
    setDesc("");
    setError("");
    setFormOpen(false);
  };

  return (
    <div>
      <div data-enter className="flex items-end justify-between gap-4">
        <div>
          <p className="font-mono text-xs tracking-[0.3em] text-ink/60">
            <span className="text-accent">A-05</span>
            <span className="mx-2">/</span>
            TICKETS
          </p>
          <h1 className="mt-3 font-display text-4xl font-bold tracking-tight">工单</h1>
        </div>
        <button
          type="button"
          onClick={() => setFormOpen((v) => !v)}
          className="border border-ink px-5 py-2.5 font-mono text-xs tracking-widest transition-colors hover:bg-ink hover:text-paper"
        >
          {formOpen ? "收起 −" : "新建工单 +"}
        </button>
      </div>

      {/* 新建表单 */}
      {formOpen && (
        <div data-enter className="mt-6 max-w-lg border border-ink/25 p-6">
          <div className="space-y-4">
            <div>
              <label className="mb-1 block font-mono text-[10px] tracking-[0.25em] text-ink/50">标题</label>
              <input
                value={title}
                onChange={(e) => setTitle(e.target.value)}
                className="w-full border-b border-ink/30 bg-transparent py-2 text-sm outline-none focus:border-ink"
              />
            </div>
            <div>
              <label className="mb-1 block font-mono text-[10px] tracking-[0.25em] text-ink/50">类型</label>
              <div className="flex flex-wrap gap-2">
                {["功能异常", "题目纠错", "功能建议", "账号问题"].map((t) => (
                  <button
                    key={t}
                    type="button"
                    onClick={() => setType(t)}
                    className={cn(
                      "border px-3 py-1.5 font-mono text-xs transition-colors",
                      type === t ? "border-ink bg-ink text-paper" : "border-line text-ink/60 hover:border-ink/40"
                    )}
                  >
                    {t}
                  </button>
                ))}
              </div>
            </div>
            <div>
              <label className="mb-1 block font-mono text-[10px] tracking-[0.25em] text-ink/50">描述</label>
              <textarea
                value={desc}
                onChange={(e) => setDesc(e.target.value)}
                rows={3}
                className="w-full border border-ink/30 bg-transparent p-2 text-sm outline-none focus:border-ink"
              />
            </div>
          </div>
          {error && <p className="mt-3 font-mono text-xs text-accent">{error}</p>}
          <button
            type="button"
            onClick={submit}
            className="mt-5 border border-ink bg-ink px-6 py-2.5 font-mono text-xs tracking-widest text-paper transition-colors hover:border-accent hover:bg-accent"
          >
            提交工单
          </button>
        </div>
      )}

      {/* 列表 */}
      <ul data-enter className="mt-8 border-t border-ink/40">
        {data.tickets.map((t) => (
          <TicketRow key={t.id} ticket={t} />
        ))}
      </ul>
    </div>
  );
}
