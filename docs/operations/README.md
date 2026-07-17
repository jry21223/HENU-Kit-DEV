# 运行维护文档

本目录保存可执行的部署、Smoke、支付联调与发布操作说明。

- [`deployment.md`](./deployment.md)：旧 Study 部署形态与上线检查。
- [`internal-smoke.md`](./internal-smoke.md)：内部测试和部署后 Smoke Runbook。
- [`wechat-pay-native.md`](./wechat-pay-native.md)：微信支付 Native 联调与上线准备。

Monorepo 的统一 CI/CD、灰度和回滚规范以 [`../development/engineering-release-spec.md`](../development/engineering-release-spec.md) 为准。目录中的旧部署命令仅适用于相应旧服务，不能直接推广到 Platform Core、Portal 或 QuizCraft。
