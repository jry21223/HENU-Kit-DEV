"use client";

import { useState } from "react";
import { EMAIL_DEMO_CODE } from "@/lib/auth/mock";
import { useEmailCode } from "@/components/account/use-email-code";
import { cn } from "@/lib/cn";

/** 验证码行：输入框 + 发送按钮（60s 倒计时）+ 演示码提示 */
export default function CodeField({
  email,
  value,
  onChange,
  error,
}: {
  email: string;
  value: string;
  onChange: (v: string) => void;
  error?: string;
}) {
  const { cd, send } = useEmailCode();
  const [sendErr, setSendErr] = useState("");

  return (
    <div>
      <label className="mb-1 block font-mono text-[10px] tracking-[0.25em] text-ink/50">
        邮箱验证码
      </label>
      <div className="flex gap-3">
        <input
          value={value}
          onChange={(e) => onChange(e.target.value)}
          placeholder="6 位数字"
          maxLength={6}
          className={cn(
            "min-w-0 flex-1 border-b bg-transparent py-2 font-mono text-sm tracking-[0.4em] outline-none placeholder:tracking-normal placeholder:text-ink/30",
            error ? "border-accent" : "border-ink/30 focus:border-ink"
          )}
        />
        <button
          type="button"
          disabled={cd > 0}
          onClick={() => setSendErr(send(email) ?? "")}
          className={cn(
            "shrink-0 border px-3 py-1.5 font-mono text-[11px] transition-colors",
            cd > 0
              ? "cursor-not-allowed border-line text-ink/40"
              : "border-ink/40 hover:border-ink"
          )}
        >
          {cd > 0 ? `${cd}s 后重发` : "发送验证码"}
        </button>
      </div>
      <p className="mt-1 font-mono text-[10px] text-ink/40">
        演示码：<span className="text-accent">{EMAIL_DEMO_CODE}</span>
      </p>
      {(error || sendErr) && (
        <p className="mt-1 font-mono text-[10px] text-accent">{error || sendErr}</p>
      )}
    </div>
  );
}
