# WSL 题库上传指南

WSL 主机上已准备 `~/quizcraft-uploader/` 目录，用来通过生产后端管理接口处理题库。

项目本身已经有题库工坊和后端提取接口：`/api/extract/parse`、`/api/extract/analyze`、`/api/banks/save`。WSL 里的脚本只是把这些现有接口做成命令行入口，不维护另一套解析逻辑。

## 原则：复用仓库已有功能

不要在 WSL 里临时写 `/tmp/parse_xxx.py` 这类一次性题库解析脚本，也不要让 WSL 单独维护一套解析规则。题库提取规则的唯一主线在 Git 仓库后端和题库工坊里：

- 网页端：使用“题库工坊”上传、校验、编辑、AI 解析和保存。
- 命令行：使用 `extract_bank_via_api.py` 调用现有 `/api/extract/parse` 和 `/api/extract/export`。
- 标准 JSON：使用 `upload_bank_via_api.py` 调用现有 `/api/banks/save`。
- AI 解析：使用 `analyze_bank_via_api.py` 调用现有 `/api/extract/analyze`。

如果解析结果有选项缺失、答案错位或校验报错，应优先修复仓库里的后端解析逻辑或源文件内容，再重新跑现有接口。不要在 WSL 里绕过主链路做单独补丁，否则后续网页端和脚本行为会再次分叉。

## 文件

- `~/quizcraft-uploader/upload_bank_via_api.py`：上传脚本
- `~/quizcraft-uploader/extract_bank_via_api.py`：调用现有 `/api/extract/parse` 的提取脚本
- `~/quizcraft-uploader/analyze_bank_via_api.py`：调用现有 `/api/extract/analyze` 的 AI 解析脚本
- `~/quizcraft-uploader/.env`：本机私有配置，包含 `ADMIN_TOKEN`
- `~/quizcraft-uploader/README.md`：WSL 侧同款说明

`.env` 不要提交到 Git，也不要发到聊天里。

## 从原始文件提取

如果手里是 PDF、Word、TXT 或非标准 JSON，优先走现有后端提取接口：

```bash
cd ~/quizcraft-uploader
./extract_bank_via_api.py /tmp/source.docx --key my_bank --name "题库名称" --color "#c62828" --output /tmp/my_bank.parsed.json
```

确认输出没有问题后，可以直接保存到线上题库：

```bash
./extract_bank_via_api.py /tmp/source.docx --key my_bank --name "题库名称" --color "#c62828" --save
```

如果脚本报告选项数、答案类型等问题，不要直接 `--allow-issues` 保存；先回到源文件或输出 JSON 修正。这一步就是为了避免再次出现“答案和题目错位但脚本静默成功”的情况。

## 上传标准 JSON

```bash
cd ~/quizcraft-uploader
./upload_bank_via_api.py /tmp/mayuan_bank_v2.json --key mayuan
```

常用参数：

```bash
./upload_bank_via_api.py /tmp/new_bank.json --key my_bank
./upload_bank_via_api.py /tmp/new_bank.json --key my_bank --name "显示名称"
./upload_bank_via_api.py /tmp/new_bank.json --key my_bank --color "#c62828"
./upload_bank_via_api.py /tmp/new_bank.json --key my_bank --no-overwrite
```

默认会覆盖同 key 题库。加 `--no-overwrite` 后，如果 key 已存在会失败。

## JSON 要求

上传文件应包含：

```json
{
  "meta": {
    "name": "题库名称",
    "color": "#c62828"
  },
  "questions": []
}
```

每道题至少需要：

- `id`
- `type`: `single`、`multi` 或 `judge`
- `content`
- `options`: 选择题需要
- `answer`
- `analysis`: 可选
- `chapter`: 推荐

上传成功后，脚本会输出上传后的 `key`、`name`、`total` 和后端保存文件路径。

## 生成 AI 解析

先在 WSL 里配置一次 LLM Key：

```bash
cd ~/quizcraft-uploader
nano .env
```

保留已有 `ADMIN_TOKEN`，补齐下面几项：

```bash
LLM_PROVIDER=deepseek
LLM_MODEL=deepseek-chat
LLM_API_KEY=你的 API Key
```

如果使用 OpenAI 兼容网关，可以改成：

```bash
LLM_PROVIDER=openai
LLM_API_URL=https://your-compatible-endpoint.example/v1
LLM_MODEL=gpt-4o-mini
LLM_API_KEY=你的 API Key
```

建议先跑 3 题冒烟：

```bash
./analyze_bank_via_api.py /tmp/mayuan_bank_v2.json --key mayuan --limit 3 --output /tmp/mayuan_bank_v2.sample.json
```

检查样例输出：

```bash
python3 - <<'PY'
import json
data = json.load(open('/tmp/mayuan_bank_v2.sample.json', encoding='utf-8'))
for q in data['questions'][:3]:
    print(q['id'], q.get('analysis', '')[:120])
PY
```

确认解析质量没问题后再跑全量并保存回线上题库：

```bash
./analyze_bank_via_api.py /tmp/mayuan_bank_v2.json --key mayuan --output /tmp/mayuan_bank_v2.analyzed.json --save
```

脚本会默认拦截后端模拟解析和失败解析，避免把无效解析覆盖到线上。
