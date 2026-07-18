---
status: accepted
---

# QuizCraft 保留 React 前端并重构为 Go 后端

QuizCraft 重构保留 React + TypeScript 前端，迁入 pnpm Workspace 并围绕 Practice Core、收藏、排行、反馈和 Question Bank Workshop 收敛；现有 Python/FastAPI 单文件后端替换为模块化 Go 服务。迁移采用 API-first、稳定题目 ID、并行校验和可回滚切换，不在一次提交中重写前端、后端、数据与生产流量。
