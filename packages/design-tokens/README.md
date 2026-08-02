# HENU Kit Design Tokens

本目录提供跨 Next.js、Vue 和 React/Vite 使用的框架无关设计变量。

- `tokens.css`：浏览器可直接消费的 CSS Custom Properties。
- `tokens.json`：供构建脚本、设计工具和校验器读取的机器格式。

## 使用

在应用全局样式中引入：

```css
@import "../../../packages/design-tokens/tokens.css";
```

具体相对路径由应用构建结构决定。正式接入时应通过 workspace package 或构建复制实现，避免生产环境依赖仓库相对路径。

```css
.primary-button {
  min-height: var(--hk-touch-target);
  color: var(--hk-paper);
  background: var(--hk-ink);
  border-radius: var(--hk-radius-control);
}

.primary-button:hover {
  background: var(--hk-accent);
}
```

## 约束

- 品牌主色是“强调橙”（`--hk-accent`），不是河南大学官方标准色。
- 视觉基准是线上主站 `henukit.cn` 的工程图纸体系（纸白/墨色/强调橙）；Console 与主站共享同一 token 集，不发展独立主题。
- 业务页不得复制 token 后长期分叉。
- 新 token 必须有至少两个真实消费场景，或属于语义状态基线。
- 破坏性重命名需要 Design System 主版本升级。
- `prefers-reduced-motion` 下非必要动画时长归零。

规范来源：[`../../docs/product/DESIGN_SYSTEM.md`](../../docs/product/DESIGN_SYSTEM.md)。
