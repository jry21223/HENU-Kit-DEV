/**
 * 全站法律与主体信息的唯一出处：页脚、登录与支付前的声明、协议页都从这里取，
 * 改一处即全站生效（DESIGN_SYSTEM §16）。
 */

/** 空间有限时使用的短版声明。 */
export const SITE_DISCLAIMER_SHORT = "学生自主运营 · 非河南大学官方项目";

/** 登录、支付等操作前再次说明主体时使用的一句话。 */
export const SITE_OPERATOR_STATEMENT = "HENU Kit 由学生自主运营，非河南大学官方项目。";

/**
 * henukit.cn 的网站备案号，须与工信部备案系统（beian.miit.gov.cn）中的记录逐字一致。
 * 设为 null 时页脚不展示任何占位文字。
 */
export const ICP_FILING: string | null = "苏ICP备2025220034号-3";

export const ICP_FILING_URL = "https://beian.miit.gov.cn/";

export const LEGAL_LINKS = [
  { href: "/terms", label: "用户协议" },
  { href: "/privacy", label: "隐私政策" },
] as const;

/** 隐私政策与用户协议的最近更新日期；内容有变更时一并更新。 */
export const LEGAL_UPDATED_AT = "2026 年 9 月 25 日";
