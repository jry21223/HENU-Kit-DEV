# HENU Kit 资料库同步(HENU-Final-Review → henukit.cn)

资料库内容来自公开仓库 [jry21223/HENU-Final-Review](https://github.com/jry21223/HENU-Final-Review)。
仓库 `manifest.json` 是唯一事实来源:它给每个资产标注了 role(复习讲义 / 课件PPT /
往年真题 / 题库练习 / 笔记总结 / 待复核资料)、sha256 与字节数。

## 架构

```
GitHub push (HENU-Final-Review, refs/heads/main)
        │  webhook (secret 校验 / 队列 / 去重,deploy-webhook 第二实例)
        ▼
henukit-materials-webhook.service (127.0.0.1:10088)
        │  systemd path 触发
        ▼
henukit-materials-runner.service (root, oneshot)
        │  henukit-materials-sync.sh
        ├─ 1. sync-henukit-materials.sh   git clone/fetch → 只镜像非"待复核"资产,
        │     原子换入 /opt/henukit-materials/public(nginx 只服务这个目录)
        ├─ 2. convert-henukit-slides.py   "课件PPT" → 门户 Slides JSON
        │     (python-pptx;.ppt 先经 LibreOffice 转 pptx)
        └─ 3. import-henukit-materials.mjs   manifest → courses/materials 幂等 upsert
              (school/college/major/course 规范化;storage_key 唯一索引;下线已移除资产)
```

- **nginx(容器内)** 以只读卷挂载 `$HENUKIT_MATERIALS_ROOT/public` 到 `/srv/materials`,
  `/materials/` 前缀禁止目录列举、拒绝点文件、强制下载(`Content-Disposition: attachment`)。
  git checkout 与仓库工具目录**永远不在**服务目录内,`/.git/` 不可能被暴露。
- **portal-api** 从遗留 Study 库读 `courses`/`materials`(新增 `sha256`/`slides` 列),
  列表接口带 `filePath`/`fileSize`,`GET /api/v1/library/materials/{id}` 详情带 `slides`。
- **portal** 资料详情页:slides 类型主操作"浏览幻灯片"(`/library/slides/[id]`,
  浏览器内翻页浏览,无需下载 5–19MB 的 PPT),其余资料提供原文件下载入口。

## 首次部署(服务器,root)

前置:GitHub Actions 已部署过的宿主(HENU Kit release),`git`、`python3`、
`python3-pptx`、`node`(≥18)、docker CLI;处理历史 `.ppt` 还需要 `libreoffice-impress`
(仅转换器用到,缺失时文件同步照常,幻灯片转换跳过并告警)。

```bash
# 1. 安装 webhook 第二实例(复用 deploy-webhook 二进制与安装器)
sudo services/deploy-webhook/deploy/install.sh --enable-materials-sync
#    生成 /etc/henukit-deploy/materials-webhook.env 与 materials-webhook-secret

# 2. 检查 env:数据库走 compose postgres 时无需改动;
#    若 STUDY_DATABASE_URL 指向其他实例,设置 HENUKIT_MATERIALS_DATABASE_URL
#    (需主机上有 psql)。
cat /etc/henukit-deploy/materials-webhook.env

# 3. 首次同步 + 导入(webhook 之外先手动跑一遍)
HENUKIT_MATERIALS_ROOT=/opt/henukit-materials \
  /usr/local/libexec/henukit/henukit-materials-sync.sh
# 预期:mirrored 182 assets, skipped 66 pending review;slides converted ~90;
#      导入 summary imported 182

# 4. 校验
curl -sI https://henukit.cn/materials/高等数学A（二）/复习讲义/高等数学A（二）_考前复习知识点讲义.pdf | head -5   # 200 + attachment
curl -s https://henukit.cn/api/v1/library/materials | head -c 300
curl -s https://henukit.cn/api/v1/library/courses | head -c 300

# 5. 配置 host 级 nginx(与 /webhooks/github 同一 HTTPS 段):
#    复制 services/deploy-webhook/deploy/nginx-materials.conf.example

# 6. GitHub 仓库设置 → Webhooks → 添加:
#    Payload URL: https://henukit.cn/webhooks/materials
#    Content type: application/json;Secret: /etc/henukit-deploy/materials-webhook-secret 的内容
#    Events: 只勾 push

# 7. 发一个 Ping 与一次真实 push,确认队列落盘,然后启用队列触发器:
systemctl enable --now henukit-materials-webhook.path
journalctl -u henukit-materials-runner.service -f
```

## 幂等与安全性质

- 重跑安全:同步按 manifest 重建镜像,导入按 `storage_key` 唯一索引 upsert;
  幻灯片按源文件 mtime 跳过已转换文件。
- 下线一致性:manifest 中移除的资产会同时从镜像消失并在库中置 `archived`
  (仅限带 `sha256` 标记的镜像行,不影响遗留资料)。
- 路径安全:manifest 路径在 Python 与 SQL 两侧都做了越界检查;镜像目录
  不含点文件;`/materials/.` 由 nginx 显式 404。
- 内容安全:第三方文档一律 `nosniff` + `Content-Disposition: attachment` +
  CSP `default-src 'none'; sandbox`,任何文档都不会在我们的源站以 HTML 执行。
- 待复核资料(role 以"待复核"开头)在镜像、转换、导入三个环节全部跳过,
  与仓库 PUBLICATION_POLICY.md 保持一致。

## 手动命令速查

```bash
# 仅镜像文件
sudo /usr/local/libexec/henukit/sync-henukit-materials.sh
# 仅转幻灯片
sudo python3 /usr/local/libexec/henukit/convert-henukit-slides.py \
  --mirror /opt/henukit-materials/public --out /opt/henukit-materials/slides \
  --manifest /opt/henukit-materials/repo/manifest.json
# 仅导入(本地调试)
node scripts/ops/import-henukit-materials.mjs --manifest manifest.json \
  | psql "$STUDY_DATABASE_URL" -v ON_ERROR_STOP=1 -f -
# 状态
curl -s http://127.0.0.1:10088/statusz
```

## 测试

```bash
bash -n scripts/ops/sync-henukit-materials.sh scripts/ops/henukit-materials-sync.sh services/deploy-webhook/deploy/install.sh
python3 -m py_compile scripts/ops/convert-henukit-slides.py
node --test scripts/ops/tests/import-henukit-materials.test.mjs
cd services/deploy-webhook && go test -race ./... && go vet ./...
```
