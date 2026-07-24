# 运行维护文档

本目录保存可执行的部署、Smoke、支付联调与发布操作说明。

- [`PRODUCTION_RELEASE_CHECKLIST.md`](./PRODUCTION_RELEASE_CHECKLIST.md)：全套服务上线前唯一的 Go/No-Go 汇总检查表；GitHub Issue 关闭不替代其中任何验收证据。
- [`deployment.md`](./deployment.md)：旧 Study 部署形态与上线检查。
- [`internal-smoke.md`](./internal-smoke.md)：内部测试和部署后 Smoke Runbook。
- [`wechat-pay-native.md`](./wechat-pay-native.md)：微信支付 Native 联调与上线准备。

Monorepo 的统一 CI/CD、灰度和回滚规范以 [`../development/engineering-release-spec.md`](../development/engineering-release-spec.md) 为准。目录中的旧部署命令仅适用于相应旧服务，不能直接推广到 Platform Core、Portal 或 QuizCraft。

生产批准必须同时绑定精确 Release SHA、CI、Migration/恢复、跨浏览器、真实依赖、部署后 Smoke 和观察期证据。