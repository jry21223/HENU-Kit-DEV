"use client";

import Link from "next/link";
import { useEffect, useState } from "react";

const STORAGE_KEY = "henukit-qq-binding";
const TOKEN = /^[A-Za-z0-9_-]{43}$/;

type Binding = { bound: boolean; display_name?: string };
class BindingLoginRequired extends Error {}

async function bindingRequest(action: "status" | "authorize" | "unlink", token = "") {
  const response = await fetch(`/api/v1/account/qq-binding/${action}`, {
    method: "POST", credentials: "same-origin", cache: "no-store",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(token ? { token } : {}),
  });
  const envelope = await response.json();
  if (!response.ok) {
    if (response.status === 401 || (response.status === 403 && envelope.error?.code === "BINDING_FORBIDDEN")) throw new BindingLoginRequired("登录已失效，请重新登录 HENU KIT。");
    throw new Error(typeof envelope.error?.message === "string" ? envelope.error.message : "绑定服务暂时不可用，请稍后重试。");
  }
  return envelope.data;
}

export default function QQBindingPage() {
  const [token, setToken] = useState("");
  const [signedIn, setSignedIn] = useState(false);
  const [name, setName] = useState("");
  const [binding, setBinding] = useState<Binding | null>(null);
  const [busy, setBusy] = useState(true);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const [unlinkPrompt, setUnlinkPrompt] = useState(false);

  useEffect(() => {
    let cancelled = false;
    // Fragment tokens never enter HTTP paths, Referer, or OAuth return_to.
    // Only retain this non-authorizing link for the same tab's login roundtrip.
    const fragment = window.location.hash.slice(1);
    let recoveredToken = "";
    try {
      if (TOKEN.test(fragment)) {
        sessionStorage.setItem(STORAGE_KEY, JSON.stringify({ token: fragment, expires: Date.now() + 300_000 }));
      } else if (fragment) {
        sessionStorage.removeItem(STORAGE_KEY);
      }
      const saved = JSON.parse(sessionStorage.getItem(STORAGE_KEY) || "null");
      if (saved && TOKEN.test(saved.token) && saved.expires > Date.now()) recoveredToken = saved.token;
      else sessionStorage.removeItem(STORAGE_KEY);
    } catch {
      // Storage can be disabled; the current fragment still works while signed in.
      if (TOKEN.test(fragment)) recoveredToken = fragment;
    }
    window.history.replaceState(null, "", "/bind/qq");
    void (async () => {
      try {
        const response = await fetch("/api/v1/session", { credentials: "same-origin", cache: "no-store" });
        if (response.status === 401) return;
        if (!response.ok) throw new Error("登录状态暂时无法读取，请稍后刷新重试。");
        const current = await response.json();
        if (!current.user_id) return;
        const status: Binding = await bindingRequest("status");
        if (!cancelled) {
          setSignedIn(true);
          setName(typeof current.display_name === "string" ? current.display_name : "当前账号");
          setBinding(status);
        }
      } catch (cause) {
        if (!cancelled) setError(cause instanceof Error ? cause.message : "暂时无法读取绑定状态，请稍后重试。");
      } finally {
        if (!cancelled) { setToken(recoveredToken); setBusy(false); }
      }
    })();
    return () => { cancelled = true; };
  }, []);

  async function act(action: "authorize" | "unlink") {
    setBusy(true); setError(""); setMessage("");
    try {
      await bindingRequest(action, action === "authorize" ? token : "");
      if (action === "authorize") setMessage("网页授权完成。请回到原 QQ 私聊，核对账号后回复“确认”，完成绑定。");
      else { setBinding({ bound: false }); setMessage("已解绑。HENU Bot 后续操作不再使用此账号授权。"); }
      setToken(""); setUnlinkPrompt(false);
      try { sessionStorage.removeItem(STORAGE_KEY); } catch { /* optional storage */ }
    } catch (cause) {
      if (cause instanceof BindingLoginRequired) { setSignedIn(false); setBinding(null); }
      setError(cause instanceof Error ? cause.message : "操作未完成，请稍后重试。");
    } finally { setBusy(false); }
  }

  return (
    <main className="bg-paper px-6 py-16 text-ink">
      <section className="mx-auto max-w-lg border border-ink/25 p-8">
        <Link href="/account/security" className="text-sm underline">返回账号安全</Link>
        <h1 className="mt-6 font-display text-3xl font-bold">绑定 HENU Bot</h1>
        <p className="mt-4">绑定后，HENU Bot 可以识别你的 HENU KIT 账号。绑定不等于登录学校 IDS 或雨课堂。</p>
        <p className="mt-3 text-sm">只授权你本人在 QQ 私聊中发起的请求，不要授权别人转发的链接。链接五分钟内有效。</p>
        {busy && <p className="mt-4" role="status">正在处理…</p>}
        {error && <p className="mt-4 text-red-700" role="alert">{error}</p>}
        {message && <p className="mt-4" role="status">{message}</p>}
        {!busy && !signedIn && <a className="mt-6 inline-block border px-5 py-3" href="/api/v1/auth/login?return_to=%2Fbind%2Fqq">登录 HENU KIT</a>}
        {signedIn && <p className="mt-5">当前账号：{name}</p>}
        {signedIn && token && !message && <button disabled={busy} onClick={() => void act("authorize")} className="mt-6 border border-ink bg-ink px-5 py-3 text-paper disabled:opacity-50">授权绑定当前账号</button>}
        {!token && !message && <p className="mt-5">请在 QQ 私聊 HENU Bot 发送“绑定 HENU KIT”，获取新的绑定链接。</p>}
        {binding?.bound && !unlinkPrompt && <button disabled={busy} onClick={() => setUnlinkPrompt(true)} className="mt-6 block underline">解除 QQ 绑定</button>}
        {unlinkPrompt && <div className="mt-6 border p-4"><p>解绑后，此 QQ 将无法继续以你的账号操作 HENU KIT。确定解绑吗？</p><button disabled={busy} onClick={() => void act("unlink")} className="mt-3 border px-4 py-2">确认解绑</button><button disabled={busy} onClick={() => setUnlinkPrompt(false)} className="ml-4 underline">取消</button></div>}
      </section>
    </main>
  );
}
