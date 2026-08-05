# 运行维护文档

本目录保存可执行的部署、Smoke、支付联调与发布操作说明。

- [`deployment.md`](./deployment.md)：旧 Study 部署形态与上线检查。
- [`henukit-artifact-deployment.md`](./henukit-artifact-deployment.md)：HENU Kit GitHub 固定 SHA 成品部署、迁移与旧运行时清退。
- [`notice-food-production-onboarding.md`](./notice-food-production-onboarding.md)：Notice / Food 服务生产编排接入、配置、发布、回滚与验证。
- [`github-webhook-deploy.md`](./github-webhook-deploy.md)：Monorepo GitHub Webhook 自动同步、精确 SHA 发布、Systemd/Nginx、批准与回滚 Runbook。
- [`PRODUCTION_RELEASE_CHECKLIST.md`](./PRODUCTION_RELEASE_CHECKLIST.md)：整套服务生产发布唯一 Go/No-Go 汇总入口。
- [`PRODUCTION_RELEASE_REPORT-2026-07-24.md`](./PRODUCTION_RELEASE_REPORT-2026-07-24.md)：本次 Webhook 交付与最终生产发布证据审计，当前结论按证据更新。
- [`internal-smoke.md`](./internal-smoke.md)：内部测试和部署后 Smoke Runbook。
- [`wechat-pay-native.md`](./wechat-pay-native.md)：微信支付 Native 联调与上线准备。

Monorepo 的统一 CI/CD、灰度和回滚规范以 [`../development/engineering-release-spec.md`](../development/engineering-release-spec.md) 为准。目录中的旧部署命令仅适用于相应旧服务，不能直接推广到 Platform Core、Portal 或 QuizCraft。
