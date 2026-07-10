# libraryctl Roadmap

## V1 — 已实现（2026-06-30）

| 命令 | 状态 | 说明 |
|---|---|---|
| `init-root` | ✅ | 初始化资料库根目录骨架 |
| `init-course` | ✅ | 初始化单门课程目录 + course.yaml + materials.csv |
| `validate` | ✅ | 校验目录完整性、YAML 必填字段、CSV 有效性、文件路径 |
| `export-web` | ✅ | 遍历所有课程生成网页后台导入 JSON manifest |

### V1.1 — 材料路径边界（2026-07-10）

- `materials.csv` 的 `path` 只接受课程目录内的相对路径。
- POSIX/Windows 绝对路径、盘符、UNC、NUL 和独立 `..` 路径段会被拒绝。
- 已存在文件会检查真实路径，拒绝通过符号链接逃逸课程目录。
- `validate` 与 `export-web` 共用同一个路径安全 helper；直接导出不会绕过边界。
- manifest 中的合法路径统一使用 `/` 分隔符，导出文件采用临时文件后 rename 的替换方式。
- `npm run test:libraryctl` 覆盖正常路径、跨平台危险输入、符号链接与失败导出不破坏旧文件。

---

## V2 — 规划中

### 2.1 `scan` — 资料库扫描

```
libraryctl scan --root ./资料库
```

输出 JSON 统计报告：

```json
{
  "courses": 12,
  "materials": 258,
  "missingIndexRows": 8,
  "untrackedFiles": 17,
  "possibleDuplicates": 4,
  "errors": []
}
```

**实现要点：**
- 遍历 `02_学校库/` 下所有课程
- 对比 `materials.csv` 登记文件 vs 实际文件系统文件
- 统计每门课的 `raw / pending / reviewed / published / archived` 数量
- 标记未登记文件（存在于磁盘但不在 CSV 中）
- 标记悬挂引用（CSV path 指向不存在的文件）

**依赖：** 无新依赖，纯 Node.js `fs.readdirSync` 递归遍历

**工作量：** ~150 行

---

### 2.2 `normalize` — 文件命名规范化

```
libraryctl normalize --root ./资料库 --dry-run   # 预览
libraryctl normalize --root ./资料库 --apply       # 执行
```

**实现要点：**
- 第一版只处理 `materials.csv` 中已登记且 path 存在的文件
- 调用 `normalizer.buildMaterialFileName()` 生成规范名
- `--dry-run` 输出改名计划 JSON
- `--apply` 执行 `fs.renameSync` 并更新 CSV 中的 path
- 处理冲突：如果目标文件名已存在，追加序号后缀 `_2`

**边界情况：**
- 跨目录改名（如从 `02_原始资料/98_图片资料/` 移到 `04_原始资料/90_其他资料/`）——V2.1 暂不做，仅同目录改名
- 未登记文件——跳过，输出 warning

**依赖：** `normalizer.mjs`（已实现核心函数）

**工作量：** ~200 行

---

### 2.3 `hash` — SHA256 计算与去重检测

```
libraryctl hash --root ./资料库
```

**实现要点：**
- 遍历所有课程的 `materials.csv`
- 对 `sha256` 为空的条目，读取文件计算 SHA256
- 写回 `materials.csv`
- 按 SHA256 分组，标记相同哈希的文件组
- 输出可能重复列表

**性能考虑：**
- 跳过超大文件（> 500MB）并记录 warning
- 大文件可使用流式哈希（`fs.createReadStream` + `crypto.createHash`）
- 支持 `--course` 参数只处理单门课程

**依赖：** `hasher.mjs`（`hashFile` 已实现，`hashAll` 待实现）

**工作量：** ~200 行

---

### 2.4 `dedupe` — 去重

```
libraryctl dedupe --root ./资料库 --dry-run   # 预览
libraryctl dedupe --root ./资料库 --apply       # 执行
```

**实现要点：**
- 依赖 `hash` 命令的输出或直接调用 `hasher.hashFile`
- 只处理完全相同 SHA256 的文件
- 保留策略：优先保留路径更规范（包含课程名）、文件名更短的文件
- 重复文件移至 `99_课程归档/01_重复文件/`
- 记录到 `00_课程档案/重复文件记录.csv`
- `--apply` 才实际移动文件

**重复文件记录 CSV 字段：**
```
sha256, kept_path, removed_path, removed_original_name, dedupe_date
```

**依赖：** `hasher.mjs`、`dedupe.mjs`（待实现）、`materials.mjs`

**工作量：** ~250 行

---

### 2.5 增强 `validate`

| 检查项 | 说明 |
|---|---|
| 命名规范检查 | 检查 CSV 登记的 path 文件名是否符合命名规范 |
| 未登记文件警告 | 对比文件系统和 CSV 登记 |
| 跨课程重复 | 不同课程间相同 SHA256 的文件 |
| course.yaml 完整性 | 增加更多字段校验 |
| 编号连续性 | 检查 local_id 是否连续无跳号 |

---

### 2.6 增强 `export-web`

| 功能 | 说明 |
|---|---|
| 增量导出 | 基于 `web_id` 判断新增/更新/删除 |
| 文件 base64 | 可选内嵌小文件（< 1MB）内容 |
| 导出 CSV | 兼容旧版资料库索引 CSV 格式 |
| 统计摘要 | 按学校/学院/阶段的汇总信息 |

---

## V3+ — 未来方向

| 功能 | 说明 |
|---|---|
| `import-legacy` | 从旧版扁平目录批量迁移到新结构 |
| `watch` | 监听文件变化自动更新 materials.csv |
| `serve` | 启动本地预览服务器 |
| `diff` | 两个资料库版本间的差异对比 |
| `backup` | 打包归档 |
| Web UI 集成 | 在 Admin 后台嵌入 libraryctl 功能 |
| CI 集成 | GitHub Actions 自动 validate + export-web |

---

## 优先级

```
P0 (今天): V1 4 个命令 ✅
P1 (本周): scan + normalize
P2 (下周): hash + dedupe
P3 (后续): validate 增强 + export-web 增强
P4 (远期): V3 功能
```
