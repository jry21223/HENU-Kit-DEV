---
status: accepted
---

# HENUKit 新前端 Workspace 使用 pnpm

HENUKit Console、Portal、拆出的 Study Legacy Admin 和重构后的 QuizCraft React 前端使用 pnpm 管理依赖，以获得更严格的依赖边界、共享依赖存储和更适合 Monorepo 的安装方式。迁移必须作为独立、可验证的工程步骤处理，使用 `pnpm-workspace.yaml` 与 `pnpm-lock.yaml`，更新本地命令和 CI；不得在未验证全部受影响前端构建兼容性的情况下直接删除现有 npm lockfile。
