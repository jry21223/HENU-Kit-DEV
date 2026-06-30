# libraryctl

本地资料库规范化工具，用于初始化课程目录、校验索引、规范文件命名、去重，并生成可导入网页后台的资料清单。

## 定位

libraryctl 是**本地目录 → 网页资料库**之间的桥。它不连接数据库、不上传文件、不调用 AI。它只操作本地文件系统。

## 快速开始

```bash
# 初始化资料库根目录
node scripts/libraryctl/libraryctl.mjs init-root --root ./资料库

# 初始化一门课程
node scripts/libraryctl/libraryctl.mjs init-course \
  --root ./资料库 \
  --school 河南大学 \
  --college 软件学院 \
  --stage 大一 \
  --semester 下学期 \
  --course 离散数学

# 校验资料库
node scripts/libraryctl/libraryctl.mjs validate --root ./资料库

# 生成网页导入清单
node scripts/libraryctl/libraryctl.mjs export-web \
  --root ./资料库 \
  --out ./dist/material-import-manifest.json
```

## 命令

### V1（已实现）

| 命令 | 说明 |
|---|---|
| `init-root` | 初始化资料库根目录结构（00_模板、01_收件箱、02_学校库、90_公共资料、99_全局归档） |
| `init-course` | 初始化单个课程目录，生成 course.yaml + materials.csv + 全部子目录 |
| `validate` | 校验目录结构完整性、course.yaml 必填字段、materials.csv 数据有效性、文件路径是否存在 |
| `export-web` | 遍历所有课程，生成可导入网页后台的 JSON manifest |

### V2（规划中）

| 命令 | 说明 |
|---|---|
| `scan` | 扫描资料库统计（课程数、资料数、未登记文件、可能重复） |
| `normalize` | 规范文件命名（按 课程名_类型_年份_标题.扩展名 规则） |
| `hash` | 计算 SHA256，补全 materials.csv 中缺失的 sha256 字段 |
| `dedupe` | 按 SHA256 去重，重复文件移至 99_课程归档/01_重复文件/ |

## 资料库结构

```
{root}/
  00_模板/
  01_收件箱/
    00_待分类/
    01_待去重/
    02_待确认课程/
  02_学校库/
    01_河南大学/
      01_软件学院/
        00_学院公共资料/
        01_大一/
          01_上学期/
          02_下学期/
            01_高等数学A（二）/
            02_离散数学/
              ├── 00_课程档案/
              │   ├── course.yaml
              │   └── materials.csv
              ├── 01_发布区/
              │   ├── 01_成品复习包/
              │   ├── 02_单项可下载资料/
              │   └── 90_生成源文件/
              ├── 02_知识点库/
              ├── 03_题库/
              ├── 04_原始资料/
              │   ├── 01_真题样卷/
              │   ├── 02_课件讲义/
              │   ├── 03_题库练习/
              │   ├── 04_教材题解/
              │   ├── 05_笔记总结/
              │   ├── 06_实验代码/
              │   └── 90_其他资料/
              ├── 05_处理中/
              │   ├── 01_待OCR/
              │   ├── 02_待清洗/
              │   ├── 03_待补全信息/
              │   └── 04_待审核/
              └── 99_课程归档/
                  └── 01_重复文件/
        02_大二/
        03_大三/
        04_大四/
  90_公共资料/
  99_全局归档/
```

## 模块

```
libraryctl/
  libraryctl.mjs        CLI 入口
  lib/
    paths.mjs           目录路径生成与发现
    course.mjs          course.yaml 读写
    materials.mjs       materials.csv 读写
    scanner.mjs         文件扫描 (V2)
    validator.mjs       结构与数据校验
    normalizer.mjs      文件名规范化
    hasher.mjs          SHA256 计算 (V2)
    dedupe.mjs          去重 (V2)
    export-web.mjs      网页导入清单生成
  schemas/
    course.schema.json
    materials.schema.json
  examples/
    course.yaml
    materials.csv
```

## 设计原则

- **零外部依赖**：纯 Node.js 内置模块
- **目录按培养方案阶段走，文件按资料年份走**
- **JSON 输出**：所有命令默认输出 JSON，方便管道与脚本集成
- **保守操作**：默认不修改文件，需要 `--apply` 才写入
