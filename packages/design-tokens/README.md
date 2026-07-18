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
  color: white;
  background: var(--hk-ink-green);
  border-radius: var(--hk-radius-control);
}

.primary-button:hover {
  background: var(--hk-ink-green-deep);
}
```

## 约束

- `--hk-ink-green` 的产品名称是“Kit 墨绿”，不是河南大学官方标准色。
- 业务页不得复制 token 后长期分叉。
- 新 token 必须有至少两个真实消费场景，或属于语义状态基线。
- 破坏性重命名需要 Design System 主版本升级。
- `prefers-reduced-motion` 下非必要动画时长归零。

规范来源：[`../../docs/reference/product/DESIGN_SYSTEM.md`](../../docs/reference/product/DESIGN_SYSTEM.md)。
