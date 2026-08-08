# HENU Kit 资料库同步(HENU-Final-Review → henukit.cn)

资料库内容来自公开仓库 [jry21223/HENU-Final-Review](https://github.com/jry21223/HENU-Final-Review)。
仓库 `manifest.json` 是候选资料的事实来源：每个资产都带有 role、SHA-256 和字节数。

## #306-A 状态：仅准备，默认拒绝激活

本切片**没有启用** webhook、队列、root runner、Nginx 切换、目录镜像、数据库导入或生产
配置。不要运行旧的 `henukit-materials-sync.sh`、`sync-henukit-materials.sh`、root
systemd 安装/启用命令，或直接将导入 SQL 交给生产数据库。它们都需要后续 activation
切片的锁、回滚、审计和明确批准，当前没有安全授权路径。

旧脚本仅作为迁移期历史实现保留；它们不是 #306-A 的操作说明，也不代表任何生产能力已启用。

## 候选准备（仓库本地、非 root）

当前唯一可执行的公开边界是仓库中的 CLI：

```bash
node scripts/ops/prepare-henukit-materials.mjs \
  --repository https://github.com/jry21223/HENU-Final-Review.git \
  --ref refs/heads/main \
  --sha <accepted-lowercase-40-character-sha> \
  --candidate-dir /var/lib/henukit-materials/candidates/<accepted-sha>
```

它必须由非 root 帐户执行。CLI 只在新的候选目录中生成 detached checkout、已复核资料镜像、
Slides JSON 和 `READY`；它不接受公开目录、数据库或激活参数。默认受保护的公开根是
`/opt/henukit-materials/public`；运维可用绝对路径的
`HENUKIT_MATERIALS_PUBLIC_ROOT` 追加当前宿主的公开根。候选目录位于任一受保护根内，
或通过符号链接解析到其中，都会被拒绝。

接受的完整 ref 和 SHA 必须同时提供。CLI 抓取 ref、独立解析其 SHA，并在不匹配时停止；
它不会改用源仓库的当前默认分支。它还会在复制前验证每个已复核资产的安全相对路径、常规
文件类型、字节数、SHA-256、重复 path 和重复 SHA-256。`READY` 只表示候选准备成功，
不表示公开发布。

## 派生目录预检（不是导入批准）

`import-henukit-materials.mjs` 只生成 DML，并在 `BEGIN` 前生成只读 psql 预检。预检把
`search_path` 固定到 `pg_catalog, public`，要求 `materials.sha256`、`materials.slides`，
以及可用、ready、live 的 `materials_storage_key_active_idx` 部分唯一索引。缺少任何条件时，
psql 会在开始事务前停止，且不会尝试 `ALTER TABLE`、`CREATE INDEX`、GORM AutoMigrate 或
其他运行时 schema 修改。

遗留 Study API 目前没有经审核、随发布包交付的 migration owner。因此本切片不提供
schema 安装或数据库写入命令。后续批准的 owner 流程必须先安装前置 schema，才可在单独的
activation 切片中执行导入。

## 本地验证

```bash
node --check scripts/ops/prepare-henukit-materials.mjs
node --check scripts/ops/import-henukit-materials.mjs
node --test scripts/ops/tests/prepare-henukit-materials.test.mjs
node --test scripts/ops/tests/import-henukit-materials.test.mjs
node --test scripts/ops/tests/import-henukit-materials-preflight.test.mjs
```
