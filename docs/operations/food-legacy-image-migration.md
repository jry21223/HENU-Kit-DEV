# 旧 Portal 美食图片迁移到 Food owner

本流程只迁移已经核对的 5 张 `survey-*` 首图。发布 artifact 同时携带
`bin/import-legacy-portal-food-images.mjs` 与同 SHA 的
`bin/food-sanitize-post-image`；迁移必须使用这对文件。净化器复用 Food owner 的正常投稿边界：
完整解码、应用 JPEG EXIF orientation、重新编码并移除 EXIF/XMP/GPS/文本元数据。

## 前置门禁

1. 固定已激活的完整 HENU Kit SHA，确认 `<release>/RELEASE_SHA` 与线上镜像一致。
2. 分别对 Portal 和 Food PostgreSQL 做 custom-format 备份，执行 `pg_restore --list`，记录 SHA-256。
3. 创建 root-owned `0600` 的临时 `PGSERVICEFILE`，只定义只读 `portal` 与可写 `food` 两个 service；密码不得出现在参数、终端输出或仓库文件中。
4. 核对 Food 目标 5 个 post UUID 均存在；任何缺失都禁止补写或猜测映射。

## Dry-run 与写入

先运行默认只读模式。它读取并核对 5 张源图、逐张调用同目录 Food 净化器，但不连接目标写事务：

```bash
sudo PGSERVICEFILE=/run/henukit-food-image-migration.pgservice \
  node <release>/bin/import-legacy-portal-food-images.mjs \
  --source-service portal --target-service food
```

只有输出 `Sanitized and validated 5 ... no target writes were made` 后，才使用同一 release、同一 service 文件执行：

```bash
sudo PGSERVICEFILE=/run/henukit-food-image-migration.pgservice \
  node <release>/bin/import-legacy-portal-food-images.mjs \
  --source-service portal --target-service food --apply
```

目标写入是单事务、幂等且不覆盖已有图片：缺 post、不同 ID、类型、大小、SHA 或 bytes 都会整体回滚。

## 验收与恢复

- `food_post_images` 对 5 个固定 post 的 `position=0` 必须恰好 5 行，`byte_size`、`sha256` 与实际 `bytes` 一致。
- 逐一请求 5 个同源 `/api/v1/food/posts/<id>/images/0`，要求 `200`、正确图片 Content-Type、可解码且榜单缩略图 `naturalWidth > 0`。
- 日志和报告只记录 post/image ID、Content-Type、大小与 SHA-256，不记录原图/净化图 base64 或数据库凭据。
- 若写入前失败，无目标变更。若写入后验收失败，停止继续发布并从本次 Food 备份恢复；不要手工覆盖冲突行。恢复后再次核对 5 行计数与公开 API。
- 验收完成后删除临时 `PGSERVICEFILE`；该文件不可复用为常驻同步凭据。
