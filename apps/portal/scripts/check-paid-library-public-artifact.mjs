import { existsSync, readdirSync, readFileSync, statSync } from "node:fs";
import { dirname, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const appRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const publicArtifactRoots = [
  join(appRoot, ".next", "static"),
  join(appRoot, ".next", "server", "app"),
];

// These are post-preview paragraphs from every currently static paid material.
// They must never ship in browser assets or static route payloads before a
// server-side entitlement owner exists.
const paidFullTextMarkers = [
  "4. 求由 y = √x",
  "2025 A 卷（节选）",
  "阶梯训练 1-2（基础）",
  "篇二 示波器",
  "卷三（中等）节选",
  "线性相关 ⟺",
  "结尾段（总结）",
  "实验二要求",
];

// Download-only releases must not ship an online-preview call to action in
// either browser assets or server-rendered route payloads. The legacy route
// handlers themselves contain only redirects and therefore do not need these
// labels.
const materialBodyMarkers = ["§1 极限的定义", ...paidFullTextMarkers];
const onlinePreviewMarkers = ["立即阅读", "免费试读", "可试读", "浏览幻灯片"];
const forbiddenMarkers = [...materialBodyMarkers, ...onlinePreviewMarkers];

for (const root of publicArtifactRoots) {
  if (!existsSync(root)) {
    throw new Error(`Expected production artifact directory: ${root}`);
  }
}

function filesIn(directory) {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const file = join(directory, entry.name);
    if (entry.isDirectory()) return filesIn(file);
    return statSync(file).isFile() ? [file] : [];
  });
}

const leaked = [];
for (const root of publicArtifactRoots) {
  for (const file of filesIn(root)) {
    const contents = readFileSync(file);
    for (const marker of forbiddenMarkers) {
      if (contents.includes(Buffer.from(marker))) {
        leaked.push(`${relative(appRoot, file)} contains ${JSON.stringify(marker)}`);
      }
    }
  }
}

if (leaked.length > 0) {
  throw new Error(
    `Library online-preview content leaked into a production artifact:\n${leaked.join("\n")}`
  );
}

console.log("Library production artifacts contain no online-preview action or post-preview paid material.");
