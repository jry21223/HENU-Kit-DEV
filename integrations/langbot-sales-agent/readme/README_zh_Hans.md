# 期末复习资料销售助手

这是 final-review-platform 的 LangBot 官方 SDK 插件，用于在 QQ / 微信 / 群聊 / 私聊场景中提供受控销售入口。

当前状态：第一轮 mock 骨架。真实微信支付回调、真实发货、paid 权限和真实下载入口仍以后端 final-review-platform 为准，插件不直接处理。

## 功能

- `!资料 <课程名>`：查询课程包。
- `!购买 <课程名>`：创建 mock 微信 Native 订单并生成二维码。
- `!订单`：查看当前聊天用户最近订单。
- `!重发 <订单号>`：支付成功后重发下载入口。
- `!帮助`：查看命令说明。

也支持官方 Command 根命令：

```text
!final_review 资料 离散数学
!final_review 购买 离散数学
```

## 安全边界

插件不能：

- 处理微信支付 notify；
- 把订单改为 paid；
- 发放 entitlement；
- 直接发送 paid PDF；
- 私自改价；
- 保存微信支付商户私钥、APIv3 Key、证书或课程资料文件；
- 承诺包过、押题必中、内部资料、泄题。

## 本地调试

```bash
pip install -r requirements.txt
python -m pytest tests
lbp run
```

`lbp run` 需要 LangBot Plugin Runtime 已运行；没有 Runtime 时，测试仍可验证 mock 业务闭环。
