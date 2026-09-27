import type { FocusEvent } from "react";

/**
 * 滚动容器里的键盘焦点（DESIGN_SYSTEM §13）：子站标签行、首页手机菜单、美食五档榜单导览、
 * 账户中心菜单都是会滑动的一行或一列控件，控件又贴着容器的边。滚动容器会裁掉画在控件外面的
 * 东西，所以这些控件的焦点框画在自己里面；浏览器聚焦时只要控件露出一截就不再滚动，所以键盘
 * 聚焦的控件由 revealKeyboardFocus 整个滑进可见范围。
 */

/** 焦点框画在控件里面：墨色 2px、向内 2px，滚动容器裁不到。 */
export const INSET_FOCUS_RING =
  "focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-ink";

/**
 * 把 child 整个滑进 scroller 的可见范围（内边距以内）：被裁在哪一边，就往哪一边滑。
 * 只动 scroller 自己的 scrollLeft / scrollTop，不碰页面滚动（那归 ScrollMemory 管）；
 * scroller 没有可滚的余量时（如 md 起的标签行）什么也不做。
 */
export function revealInScroller(scroller: HTMLElement, child: Element) {
  const style = getComputedStyle(scroller);
  const box = scroller.getBoundingClientRect();
  const target = child.getBoundingClientRect();
  const left = box.left + scroller.clientLeft + parseFloat(style.paddingLeft);
  const right = box.left + scroller.clientLeft + scroller.clientWidth - parseFloat(style.paddingRight);
  const top = box.top + scroller.clientTop + parseFloat(style.paddingTop);
  const bottom = box.top + scroller.clientTop + scroller.clientHeight - parseFloat(style.paddingBottom);
  if (target.left < left) scroller.scrollLeft -= left - target.left;
  else if (target.right > right) scroller.scrollLeft += target.right - right;
  if (target.top < top) scroller.scrollTop -= top - target.top;
  else if (target.bottom > bottom) scroller.scrollTop += target.bottom - bottom;
}

/**
 * 挂在滚动容器的 onFocus 上：键盘聚焦（:focus-visible）的控件整个滑进可见范围。
 * 只管键盘焦点：点按时也滑的话，手指下的控件会在抬起前挪走。
 */
export function revealKeyboardFocus(event: FocusEvent<HTMLElement>) {
  const { target, currentTarget } = event;
  if (target !== currentTarget && target.matches(":focus-visible")) revealInScroller(currentTarget, target);
}
