"use client";

import { useEffect, useState } from "react";
import { sendEmailCode } from "@/lib/auth/mock";

/** 邮箱验证码发送 + 60s 倒计时（发送动作在事件回调中调用） */
export function useEmailCode() {
  const [cd, setCd] = useState(0);

  useEffect(() => {
    if (cd <= 0) return;
    const t = setInterval(() => setCd((v) => v - 1), 1000);
    return () => clearInterval(t);
  }, [cd]);

  /** 返回 null 表示已发送；否则为错误文案 */
  const send = (email: string): string | null => {
    if (!sendEmailCode(email)) return "邮箱格式不正确";
    setCd(60);
    return null;
  };

  return { cd, send };
}
