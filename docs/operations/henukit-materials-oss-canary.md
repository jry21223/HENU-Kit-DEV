# 私有 OSS 单份资料 canary

本流程只验证一份已经封存的审核资料能够抵达私有 OSS，并得到
`published_not_activated` 收据。它不批量发布、不切换当前资料目录、不写
Library catalog 或下载统计，也不授权生产激活。

## 固定边界

- caller 只传完整 sealed release ID、sealed receipt SHA-256 和 manifest 中
  唯一的 asset SHA-256；不要传分支、路径、Bucket、Endpoint、Object key、
  命令或凭据。
- receipt 的来源必须是固定公开免费资料仓库
  `https://github.com/jry21223/HENU-Final-Review.git` 的 `refs/heads/main`；仅
  role 不以 `待复核` 开头的常规审核文件可作为 canary，不能靠路径猜测权限。
- `/etc/henukit-deploy/materials-oss.env` 必须是 `root:root 0600` 的普通文件。
  sealed root 与 OSS audit root 必须是彼此独立、root-owned、不可被
  group/other 写入的真实目录。
- 配置只允许以下键：

  ```text
  HENUKIT_MATERIALS_SEALED_ROOT=/opt/henukit-materials/sealed
  HENUKIT_MATERIALS_OSS_AUDIT_ROOT=/opt/henukit-materials/oss-audit
  HENUKIT_MATERIALS_OSS_BUCKET=henukit
  HENUKIT_MATERIALS_OSS_REGION=cn-beijing
  HENUKIT_MATERIALS_OSS_ENDPOINT=oss-cn-beijing-internal.aliyuncs.com
  HENUKIT_MATERIALS_OSS_RAM_ROLE=henukit-materials-oss-publisher
  ```

  不要在该文件、shell history、日志或 Issue 中保存 AccessKey、Secret、STS
  token 或账户标识。
- 本票不会通过 `install.sh` 安装 wrapper/binary/config，也不会启用服务。
  下列命令只有在后续受审步骤已把固定 wrapper 与 binary 暂存到同一
  root-owned 目录后才可使用；仓库文件存在不等于生产可运行。

## Canary 验证

1. 记录运行前的 `ACTIVE_RELEASE` 与 `public/current`，并取得 #306 seal 输出
   的精确 release ID 和 receipt digest。asset digest 必须来自同一 sealed
   manifest/inventory，不能从浏览器路径或移动分支推导。
2. 只执行固定入口：

   ```sh
   sudo /usr/local/libexec/henukit/henukit-materials-publish-oss \
     --release-id <40hex>-<16hex> \
     --receipt-sha256 <64hex> \
     --asset-sha256 <64hex>
   ```

3. 成功输出必须是结构化 JSON，且 `state` 精确等于
   `published_not_activated`。保存 release、receipt、asset、Object key、
   bytes、SHA-256、publication receipt digest 和 OSS version ID；不要保存
   SDK 签名、临时 token 或完整内部响应。
4. 确认 publisher 已验证 Bucket 为北京、private、Standard、ZRS、versioning
   Enabled、SSE-OSS AES256，并完成 HEAD 与全量读取的 bytes/SHA-256 对账。
   对同一 Object key 发起未签名请求必须被拒绝。
5. 再次核对 `ACTIVE_RELEASE` 与 `public/current` 和运行前一致；不存在新的
   activation journal、maintenance fence、Library catalog 写入或 Portal
   变化。重复同一组参数应返回同一逻辑结果。

任何校验失败都表示 canary 未完成。已在远程写入前落盘的 release→receipt
身份绑定必须保留；它不是成功收据或激活记录，但会阻止另一个 receipt 在同一
release 下制造第二个孤儿对象。先保留 sealed release、错误分类和精确 OSS
version 证据进行诊断；不要改成公共 ACL、关闭加密/版本控制、切换外网 Endpoint
或注入长期 AccessKey 来绕过失败。

## 安全清理

- 只能清理 publication receipt 明确标记为 `published_not_activated`、且未被
  任何活动目录引用的 canary。
- 在 versioning Enabled 的 Bucket 中，删除时必须指定 receipt 记录的精确
  Object key 和 version ID。禁止按 `releases/`、release ID 或通配前缀批量
  删除，也不要删除 sealed receipt/inventory。
- 如果上传后、收据落盘前失败，先用精确 release/receipt/asset 派生 key，
  对照 publisher 日志与 OSS 版本记录确认唯一孤儿版本；证据不足时保留对象
  并升级人工复核，不猜测删除目标。
- 清理后重新确认当前 `ACTIVE_RELEASE`、`public/current` 和 Library catalog
  未变化。生产批量发布、激活和回滚证据属于后续 Issue。
