"use client";

import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import { gsap, FINE_MOTION } from "@/lib/gsap";
import { cn } from "@/lib/cn";

const DEMO_CODE = "427819";
const STEPS = ["验证账号", "验证码", "新密码"];

export default function RecoverPage() {
  const [step, setStep] = useState(0);
  const [account, setAccount] = useState("");
  const [code, setCode] = useState("");
  const [pwd, setPwd] = useState("");
  const [pwd2, setPwd2] = useState("");
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);
  const [done, setDone] = useState(false);
  const panelRef = useRef<HTMLDivElement>(null);

  // 步骤切换：淡入 + y 位移
  useEffect(() => {
    const mm = gsap.matchMedia();
    mm.add(FINE_MOTION, () => {
      gsap.from(panelRef.current, {
        y: 14,
        autoAlpha: 0,
        duration: 0.4,
        ease: "power2.out",
        clearProps: "all",
      });
    });
    return () => mm.revert();
  }, [step, done]);

  const nextStep = () => {
    setError("");
    if (step === 0) {
      if (!account.trim()) return setError("请输入账号名或绑定邮箱");
    } else if (step === 1) {
      if (code !== DEMO_CODE) return setError("验证码不正确（演示码 427819）");
    } else if (step === 2) {
      if (pwd.length < 6) return setError("新密码至少 6 位");
      if (pwd2 !== pwd) return setError("两次输入的密码不一致");
    }
    setPending(true);
    setTimeout(() => {
      setPending(false);
      if (step === 2) setDone(true);
      else setStep((s) => s + 1);
    }, 400);
  };

  return (
    <main className="bg-blueprint flex min-h-svh items-center justify-center px-5 py-16">
      <div className="w-full max-w-md border border-ink bg-paper p-8 md:p-10">
        <div className="flex items-baseline justify-between">
          <p className="font-mono text-xs tracking-[0.3em] text-ink/60">
            <span className="text-accent">ACC-02</span>
            <span className="mx-2">/</span>
            RECOVER
          </p>
          <Link href="/account/login" className="font-mono text-[10px] tracking-widest text-ink/40 hover:text-accent">
            ← 返回登录
          </Link>
        </div>
        <h1 className="mt-4 font-display text-4xl font-bold tracking-tight">找回密码</h1>

        {/* 步骤条 */}
        <div className="mt-6 flex items-center gap-2">
          {STEPS.map((label, i) => (
            <div key={label} className="flex flex-1 items-center gap-2">
              <span
                className={cn(
                  "flex h-6 w-6 shrink-0 items-center justify-center border font-mono text-[10px]",
                  i < step || done
                    ? "border-ink bg-ink text-paper"
                    : i === step
                      ? "border-accent text-accent"
                      : "border-line text-ink/40"
                )}
              >
                {String(i + 1).padStart(2, "0")}
              </span>
              <span className={cn("font-mono text-[10px] tracking-wider", i === step && !done ? "text-ink" : "text-ink/40")}>
                {label}
              </span>
              {i < STEPS.length - 1 && <span className="h-px flex-1 bg-line" />}
            </div>
          ))}
        </div>

        <div ref={panelRef} className="mt-8">
          {done ? (
            <div>
              <p className="border border-ink bg-ink px-4 py-3 font-mono text-sm text-paper">
                ✓ 密码已重置
              </p>
              <p className="mt-3 text-sm text-ink/60">请使用新密码重新登录。</p>
              <Link
                href="/account/login"
                className="mt-6 inline-block border border-ink px-7 py-3 font-mono text-sm tracking-widest transition-colors hover:border-accent hover:text-accent"
              >
                返回登录 →
              </Link>
            </div>
          ) : (
            <div className="space-y-5">
              {step === 0 && (
                <div>
                  <label className="mb-1 block font-mono text-[10px] tracking-[0.25em] text-ink/50">
                    账号 / 绑定邮箱
                  </label>
                  <input
                    value={account}
                    onChange={(e) => setAccount(e.target.value)}
                    placeholder="学号 / 昵称 / 邮箱"
                    className="w-full border-b border-ink/30 bg-transparent py-2 font-mono text-sm outline-none placeholder:text-ink/30 focus:border-ink"
                  />
                </div>
              )}
              {step === 1 && (
                <div>
                  <label className="mb-1 block font-mono text-[10px] tracking-[0.25em] text-ink/50">
                    验证码
                  </label>
                  <input
                    value={code}
                    onChange={(e) => setCode(e.target.value)}
                    placeholder="6 位数字"
                    maxLength={6}
                    className="w-full border-b border-ink/30 bg-transparent py-2 font-mono text-sm tracking-[0.5em] outline-none placeholder:tracking-normal placeholder:text-ink/30 focus:border-ink"
                  />
                  <p className="mt-2 border border-dashed border-ink/30 px-3 py-2 font-mono text-[10px] tracking-wider text-ink/50">
                    v1 预览不发送真实验证码，演示码：<span className="text-accent">{DEMO_CODE}</span>
                  </p>
                </div>
              )}
              {step === 2 && (
                <>
                  <div>
                    <label className="mb-1 block font-mono text-[10px] tracking-[0.25em] text-ink/50">
                      新密码
                    </label>
                    <input
                      type="password"
                      value={pwd}
                      onChange={(e) => setPwd(e.target.value)}
                      placeholder="至少 6 位"
                      className="w-full border-b border-ink/30 bg-transparent py-2 font-mono text-sm outline-none placeholder:text-ink/30 focus:border-ink"
                    />
                  </div>
                  <div>
                    <label className="mb-1 block font-mono text-[10px] tracking-[0.25em] text-ink/50">
                      确认新密码
                    </label>
                    <input
                      type="password"
                      value={pwd2}
                      onChange={(e) => setPwd2(e.target.value)}
                      className="w-full border-b border-ink/30 bg-transparent py-2 font-mono text-sm outline-none focus:border-ink"
                    />
                  </div>
                </>
              )}
              {error && <p className="font-mono text-xs text-accent">{error}</p>}
              <button
                type="button"
                onClick={nextStep}
                disabled={pending}
                className={cn(
                  "w-full border py-3.5 font-mono text-sm tracking-widest transition-colors",
                  pending
                    ? "cursor-wait border-line text-ink/40"
                    : "border-ink bg-ink text-paper hover:border-accent hover:bg-accent"
                )}
              >
                {pending ? "处理中…" : step === 2 ? "重置密码" : "下一步 →"}
              </button>
            </div>
          )}
        </div>
      </div>
    </main>
  );
}
