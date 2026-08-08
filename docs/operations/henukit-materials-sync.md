# HENU Kit 资料库同步(HENU-Final-Review → henukit.cn)

资料库内容来自公开仓库 [jry21223/HENU-Final-Review](https://github.com/jry21223/HENU-Final-Review)。
仓库 `manifest.json` 是候选资料的事实来源：每个资产都带有 role、SHA-256 和字节数。

## #306-A / B01 / C0 状态：候选准备、队列与封存边界，默认拒绝激活

本切片只定义材料实例的非 root 候选准备与队列边界，并提供一份**惰性、未安装、未接线**的
C0 root-only 源资料封存模板；它**没有启用或安装** webhook、runner、root runtime、Nginx
切换、目录镜像、公开发布、数据库导入或生产配置。C0 模板不读取候选内容，尚未安装、启用或
连接到审批/提交路径；它不能发布候选。不要运行旧的
`henukit-materials-sync.sh`、`sync-henukit-materials.sh`、root systemd 安装/启用命令，或直接
将导入 SQL 交给生产数据库。它们都需要后续 activation 切片的锁、回滚、审计和明确批准，
当前没有安全授权路径。

旧脚本仅作为迁移期历史实现保留；它们不是 #306-A、B01 或 C0 的操作说明，也不代表任何
生产能力已启用。仓库中的 unit 或 wrapper 模板同样不构成安装、root commit、公开发布或
Study 导入授权。

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

## C0 源资料封存模板（未安装、未接线）

`henukit-materials-seal` 只是仓库中的 root-only 模板，不是当前环境可执行的运维命令。它
没有安装路径、systemd unit、webhook/runner 调用、审批连接或公开/Study 提交权限，因此不要
把它复制到主机、以 root 运行，或把它当作发布流程的一步。

未来经单独审批的实现只允许用格式受限的 B01 attempt 标识作审计关联；它不会遍历、读取或
复制候选目录。固定的 root-owned 配置将提供密封根、来源仓库、完整 ref 和精确 SHA；工具从
新的 root-owned checkout 封存已复核的原始资料，并把派生 Slides 明确标为 deferred。它不会
运行 Office/LibreOffice 或解析候选 Slides。格式受限的 attempt 仅作为审计关联，持久记录在
独立的 root-owned audit record 中，不会改变 release ID、receipt digest 或 inventory。公开树、
Nginx 和 Study 目录均不在 C0 范围内。

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
node --check scripts/ops/seal-henukit-materials.mjs
bash -n services/deploy-webhook/deploy/henukit-materials-seal
node --test scripts/ops/tests/henukit-materials-seal.test.mjs
node --test scripts/ops/tests/henukit-materials-seal-wrapper.test.mjs
node --test scripts/ops/tests/henukit-materials-seal-linux.test.mjs
```
