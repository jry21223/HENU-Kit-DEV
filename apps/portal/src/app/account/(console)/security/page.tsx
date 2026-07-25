"use client";

import { useState } from "react";
import { useSyncExternalStore } from "react";
import { accountStore, EMAIL_DEMO_CODE } from "@/lib/auth/mock";
import { authStore, isMockAuthEnabled } from "@/lib/auth/store";
import CodeField from "@/components/account/code-field";
import { useReveal } from "@/components/account/use-reveal";
import { cn } from "@/lib/cn";

const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

export default function SecurityPage() {
  const data = useSyncExternalStore(accountStore.subscribe, accountStore.get, accountStore.getServer);
  const { user } = useSyncExternalStore(authStore.subscribe, authStore.get, authStore.getServer);
  const mockAuth = isMockAuthEnabled();
  useReveal();

  const [oldPwd, setOldPwd] = useState("");
  const [newPwd, setNewPwd] = useState("");
  const [newPwd2, setNewPwd2] = useState("");
  const [email, setEmail] = useState("");
  const [code, setCode] = useState("");
  const [error, setError] = useState("");
  const [ok, setOk] = useState(false);
  const [pending, setPending] = useState(false);

  const submit = () => {
    setError("");
    setOk(false);
    if (!mockAuth) {
      return setError("生产环境请通过统一认证修改密码，本地 mock 流程已禁用。");
    }
    if (!oldPwd) return setError("请输入当前密码");
    if (newPwd.length < 6) return setError("新密码至少 6 位");
    if (newPwd2 !== newPwd) return setError("两次输入的新密码不一致");
    if (!EMAIL_RE.test(email)) return setError("请输入绑定邮箱");
    if (code !== EMAIL_DEMO_CODE) return setError("验证码不正确");
    setPending(true);
    setTimeout(() => {
      setPending(false);
      setOk(true);
      setOldPwd("");
      setNewPwd("");
      setNewPwd2("");
      setCode("");
    }, 500);
  };

  return (
    <div>
      <p data-enter className="font-mono text-xs tracking-[0.3em] text-ink/60">
        <span className="text-accent">A-02</span>
        <span className="mx-2">/</span>
        SECURITY
      </p>
      <h1 data-enter className="mt-3 font-display text-4xl font-bold tracking-tight">安全设置</h1>

      <section data-enter className="mt-8 max-w-md border border-ink/25 p-6">
        <p className="font-mono text-xs tracking-[0.25em] text-ink/60">修改密码</p>
        {!mockAuth && (
          <p className="mt-3 font-mono text-[10px] leading-5 tracking-wider text-ink/50">
            当前为 Gateway 模式：不展示演示验证码，密码修改请走统一认证。
          </p>
        )}
        <div className="mt-5 space-y-4">
          {[
            { label: "当前密码", v: oldPwd, set: setOldPwd },
            { label: "新密码", v: newPwd, set: setNewPwd },
            { label: "确认新密码", v: newPwd2, set: setNewPwd2 },
          ].map((f) => (
            <div key={f.label}>
              <label className="mb-1 block font-mono text-[10px] tracking-[0.25em] text-ink/50">
                {f.label}
              </label>
              <input
                type="password"
                value={f.v}
                onChange={(e) => f.set(e.target.value)}
                disabled={!mockAuth}
                className="w-full border-b border-ink/30 bg-transparent py-2 font-mono text-sm outline-none focus:border-ink disabled:opacity-40"
              />
            </div>
          ))}
          <div>
            <label className="mb-1 block font-mono text-[10px] tracking-[0.25em] text-ink/50">
              绑定邮箱{user?.email ? `（${user.email}）` : ""}
            </label>
            <input
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder="name@stu.henu.edu.cn"
              disabled={!mockAuth}
              className="w-full border-b border-ink/30 bg-transparent py-2 font-mono text-sm outline-none placeholder:text-ink/30 focus:border-ink disabled:opacity-40"
            />
          </div>
          {mockAuth && <CodeField email={email} value={code} onChange={setCode} />}
        </div>
        {error && <p className="mt-3 font-mono text-xs text-accent">{error}</p>}
        {ok && (
          <p className="mt-3 border border-ink bg-ink px-3 py-2 font-mono text-xs text-paper">
            ✓ 密码已更新（mock）
          </p>
        )}
        <button
          type="button"
          onClick={submit}
          disabled={pending || !mockAuth}
          className={cn(
            "mt-5 border px-6 py-2.5 font-mono text-xs tracking-widest transition-colors",
            pending || !mockAuth
              ? "cursor-not-allowed border-line text-ink/40"
              : "border-ink bg-ink text-paper hover:border-accent hover:bg-accent"
          )}
        >
          {pending ? "提交中…" : mockAuth ? "确认修改" : "仅统一认证可修改"}
        </button>
      </section>

      <section data-enter className="mt-10">
        <p className="font-mono text-xs tracking-[0.25em] text-ink/60">
          登录设备 · {data.devices.length} 台
        </p>
        <div className="mt-4 border-t border-ink/40">
          {data.devices.map((d) => (
            <div
              key={d.id}
              className="flex flex-wrap items-center gap-x-6 gap-y-2 border-b border-line py-4"
            >
              <p className="font-mono text-sm">{d.name}</p>
              <p className="font-mono text-[10px] tracking-wider text-ink/50">{d.meta}</p>
            </div>
          ))}
        </div>
      </section>
    </div>
  );
}
