import type { ThemeConfig } from "antd";

/**
 * HENU Kit Console 视觉主题（遵循 DESIGN_SYSTEM.md 第 21 节）：
 * 内部运营后台不使用学生端高风格化工程图纸色板，采用克制、高清晰度、高对比度的中性灰白色阶，
 * 搭配清晰的语义状态色，优先保证长时间密集审核与操作的阅读速度与准确率。
 */
export const consoleTheme: ThemeConfig = {
  token: {
    colorPrimary: "#18181b",
    colorInfo: "#2563eb",
    colorSuccess: "#16a34a",
    colorSuccessBg: "#f0fdf4",
    colorSuccessBorder: "#bbf7d0",
    colorWarning: "#b45309",
    colorError: "#dc2626",
    colorTextBase: "#18181b",
    colorBgLayout: "#f6f7f9",
    colorBgContainer: "#ffffff",
    colorBorderSecondary: "#e5e7eb",
    controlItemBgActive: "#f1f5f9",
    controlItemBgActiveHover: "#e2e8f0",
    controlItemBgHover: "#f8fafc",
    borderRadius: 8,
    fontSize: 14,
    fontFamily:
      'ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", "Noto Sans SC", sans-serif',
  },
  components: {
    Layout: {
      siderBg: "#ffffff",
      headerBg: "#ffffff",
      bodyBg: "#f6f7f9",
      headerHeight: 56,
      headerPadding: "0 20px",
    },
    Menu: {
      itemBg: "transparent",
      itemSelectedBg: "#f1f5f9",
      itemSelectedColor: "#0f172a",
      itemHoverBg: "#f8fafc",
      itemBorderRadius: 6,
      itemHeight: 38,
      groupTitleFontSize: 12,
    },
    Card: {
      headerFontSize: 15,
      bodyPadding: 20,
    },
    Table: {
      headerBg: "#fafafa",
      headerColor: "#475569",
      rowHoverBg: "#f8fafc",
      cellPaddingBlock: 12,
      cellPaddingInline: 14,
    },
    Tabs: {
      itemSelectedColor: "#18181b",
      inkBarColor: "#18181b",
    },
    Select: {
      optionSelectedBg: "#f1f5f9",
      optionSelectedColor: "#0f172a",
      optionActiveBg: "#f8fafc",
    },
  },
};
