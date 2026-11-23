export interface FormatTimestampOptions {
  /** 是否显示秒（钱包账单等精细时间戳使用） */
  seconds?: boolean;
  /** 是否强制中国标准时间（默认 true，确保与服务端一致） */
  chinaTimeZone?: boolean;
}

/**
 * 统一的中文日期时间格式化工具，收拢账户中心多页重复实现。
 */
export function formatTimestamp(
  value: string,
  options: FormatTimestampOptions = {},
): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  const { seconds = false, chinaTimeZone = true } = options;
  return new Intl.DateTimeFormat("zh-CN", {
    ...(chinaTimeZone ? { timeZone: "Asia/Shanghai" } : {}),
    dateStyle: "medium",
    timeStyle: seconds ? "medium" : "short",
  }).format(date);
}
