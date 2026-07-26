"use client";

import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  HENU_EMAIL_SUFFIX,
  toHenuEmail,
  toHenuLocalPart,
} from "@/lib/auth/henu-email";
import { cn } from "@/lib/cn";

/**
 * Local-part only input with fixed @henu.edu.cn suffix (not editable).
 */
export function HenuEmailField({
  id = "henu-email",
  label = "学校邮箱",
  value,
  onChange,
  disabled,
  autoFocus,
}: {
  id?: string;
  label?: string;
  /** Local part only (no @domain). */
  value: string;
  onChange: (localPart: string) => void;
  disabled?: boolean;
  autoFocus?: boolean;
}) {
  const full = value ? toHenuEmail(value) : "";

  return (
    <div>
      <Label htmlFor={id}>{label}</Label>
      <div
        className={cn(
          "flex items-end gap-0 border-b border-ink/30 transition-colors focus-within:border-ink",
          disabled && "opacity-40"
        )}
      >
        <Input
          id={id}
          type="text"
          inputMode="email"
          autoComplete="username"
          autoCapitalize="none"
          autoCorrect="off"
          spellCheck={false}
          autoFocus={autoFocus}
          disabled={disabled}
          placeholder="姓名拼音 / 学号邮箱前缀"
          value={value}
          onChange={(e) => onChange(toHenuLocalPart(e.target.value))}
          className="border-0 focus:border-transparent"
          aria-describedby={`${id}-suffix`}
        />
        <span
          id={`${id}-suffix`}
          className="shrink-0 select-none pb-2 pl-1 font-mono text-sm tracking-wide text-ink/45"
        >
          {HENU_EMAIL_SUFFIX}
        </span>
      </div>
      {full ? (
        <p className="mt-1 font-mono text-[10px] tracking-wider text-ink/35">
          将发送至 {full}
        </p>
      ) : null}
    </div>
  );
}
