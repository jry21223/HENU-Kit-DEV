import type { QuizCraftSystemAvatar } from "@/lib/api/types";

/**
 * Deterministic system-avatar glyphs for the public ranking. The avatar is a
 * controlled Core enum; this component only maps the enum to a fixed SVG, so
 * an anonymous ranking can never surface an account-owned image.
 */
const AVATAR_GLYPHS: Record<
  QuizCraftSystemAvatar,
  { label: string; background: string; glyph: React.ReactNode }
> = {
  "scholar-blue": {
    label: "学者",
    background: "#3E63DD",
    glyph: (
      <>
        <path d="M18 7.6 27 11.6 18 15.6 9 11.6Z" fill="#fff" />
        <path
          d="M11.8 14v3.6c0 1.3 2.8 2.9 6.2 2.9s6.2-1.6 6.2-2.9V14"
          fill="none"
          stroke="#fff"
          strokeWidth="1.6"
          strokeLinecap="round"
        />
        <circle cx="27" cy="11.6" r="1.3" fill="#fff" />
      </>
    ),
  },
  "coder-green": {
    label: "编码者",
    background: "#2F9E63",
    glyph: (
      <>
        <path
          d="M13 9.6 8.6 16l4.4 6.4M23 9.6l4.4 6.4-4.4 6.4"
          fill="none"
          stroke="#fff"
          strokeWidth="2.2"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
        <path
          d="M20.6 8.2 15.4 23.8"
          fill="none"
          stroke="#fff"
          strokeWidth="1.7"
          strokeLinecap="round"
        />
      </>
    ),
  },
  "reader-amber": {
    label: "读者",
    background: "#D97706",
    glyph: (
      <>
        <path
          d="M9 9.2c2.7-.9 5.5-.6 8 .9 2.5-1.5 5.3-1.8 8-.9v12.4c-2.7-.9-5.5-.6-8 .9-2.5-1.5-5.3-1.8-8-.9Z"
          fill="#fff"
        />
        <path
          d="M17 10.1v12.4"
          stroke="#D97706"
          strokeWidth="1.2"
          strokeLinecap="round"
        />
      </>
    ),
  },
  "owl-purple": {
    label: "夜猫子",
    background: "#7C3AED",
    glyph: (
      <>
        <path
          d="M26 17.2a8.4 8.4 0 1 1-8.4-8.4 6.2 6.2 0 0 0 8.4 8.4Z"
          fill="#fff"
        />
        <circle cx="10" cy="25.2" r="1.2" fill="#fff" />
      </>
    ),
  },
};

export function RankingAvatar({
  avatar,
  size = 32,
  className,
}: {
  avatar: QuizCraftSystemAvatar;
  size?: number;
  className?: string;
}) {
  const item = AVATAR_GLYPHS[avatar];
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 36 36"
      role="img"
      aria-label={`系统头像：${item.label}`}
      className={className}
    >
      <title>{`系统头像：${item.label}`}</title>
      <rect width="36" height="36" rx="4" fill={item.background} />
      {item.glyph}
    </svg>
  );
}
