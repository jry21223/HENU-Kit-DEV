# 校园会员权益页面验证（2026-10-09）

页面源码：`13b0f189188edcedc663f5e6ef3101e82c7f3d14`。

以下截图通过本地 Playwright 的既有会员页面测试生成。身份、free/lifetime
资格和订单响应均为合成 fixture，没有操作真实账号、开通真实会员或产生支付。
截图展示页面权益说明及费用口径，不代表生产发布或真实平台服务验收。

- [桌面终身会员页](desktop-lifetime.png)
- [390px 终身会员页](mobile-390px-lifetime.png)
- [桌面免费会员开通说明](desktop-free-purchase.png)

对应测试为 `apps/portal/tests/account-portfolio.spec.ts` 的两个会员页面场景，
以及 `apps/portal/tests/membership-purchase.spec.ts` 的免费会员开通说明场景。
三项测试均通过；等待页面入场动画结束后截图，仅隐藏本地 Next 开发指示器，
没有修改图片或产品页面。现有 ¥9.9 价格保留。

会员页与开通说明都包含雨课堂、U校园（含 AI 版）、学习通及图书馆定时预约，
注明相关服务费用由站方承担、无需额外付费或扣除积分，并说明 QQ 绑定及
校园服务逐项开放。实际服务可用性需要配套部署及各执行端验收。
