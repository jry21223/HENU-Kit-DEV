/**
 * 层级的读者可见名字：箭头文案、各子站导航里指向同一层的落地标签、以及层级自己那
 * 一页的标题共用这一份，同一个落点不会在同一屏出现两个叫法。
 *
 * 用的是「子站导航那一套词」（书库/榜单/市集/题库…），也就是读者在落点页的标签行
 * 上正看着的那个词。首页一级入口与模块 SEO 标题用的是另一套「模块名」（资料库/
 * 美食榜/互助平台…），不在这一份里；两套语域的统一是独立事项（见 #501 的 Out of
 * Scope）。
 */
export const LEVEL_LABELS = {
  platformHome: "henukit",
  practiceBank: "题库",
  practiceFavorites: "收藏夹",
  library: "书库",
  food: "榜单",
  campus: "市集",
  career: "求职雷达",
} as const;

export type ParentRoute = {
  href: string;
  /** 箭头后面显示的层级名。 */
  label: string;
  /**
   * 读屏时说的层级名，默认同 `label`。平台首页的可见文案是品牌名：只念品牌没说清
   * 回到哪一层，但也不能丢掉屏幕上的字，否则语音控制念不出这个控件。
   */
  spokenAs?: string;
};

const PLATFORM_HOME: ParentRoute = {
  href: "/",
  label: LEVEL_LABELS.platformHome,
  spokenAs: "henukit（平台首页）",
};

/**
 * 先匹配先生效，所以更深的路径必须排在它所属模块前面（收藏夹文件夹在 /practice 之前）。
 */
const RULES: Array<{ matches: (pathname: string) => boolean; parent: ParentRoute }> = [
  {
    matches: (pathname) => pathname.startsWith("/practice/favorites/"),
    parent: { href: "/practice/favorites", label: LEVEL_LABELS.practiceFavorites },
  },
  { matches: (pathname) => pathname.startsWith("/practice/"), parent: { href: "/practice", label: LEVEL_LABELS.practiceBank } },
  { matches: (pathname) => pathname === "/practice", parent: PLATFORM_HOME },
  { matches: (pathname) => pathname.startsWith("/library/"), parent: { href: "/library", label: LEVEL_LABELS.library } },
  { matches: (pathname) => pathname === "/library", parent: PLATFORM_HOME },
  { matches: (pathname) => pathname.startsWith("/food/"), parent: { href: "/food", label: LEVEL_LABELS.food } },
  { matches: (pathname) => pathname === "/food", parent: PLATFORM_HOME },
  { matches: (pathname) => pathname.startsWith("/campus/"), parent: { href: "/campus", label: LEVEL_LABELS.campus } },
  { matches: (pathname) => pathname === "/campus", parent: PLATFORM_HOME },
  { matches: (pathname) => pathname.startsWith("/career/"), parent: { href: "/career", label: LEVEL_LABELS.career } },
  { matches: (pathname) => pathname === "/career", parent: PLATFORM_HOME },
];

/**
 * 「返回上一级」的落点：模块内的页面回到该模块首页，模块首页回到平台首页。
 *
 * 上一级按路径层级判断，而不是按浏览器历史：读者可能从任何地方进来（首页入口、
 * 搜索结果、外部链接、直接刷新），左上角那个箭头必须总是走到同一层，才算可预期。
 */
export function parentRoute(pathname: string): ParentRoute {
  return RULES.find((rule) => rule.matches(pathname))?.parent ?? PLATFORM_HOME;
}
