import { existsSync, readdirSync, readFileSync, statSync } from "node:fs";
import { dirname, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const appRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const staticAssetRoot = join(appRoot, ".next", "static");
const serverAppRoot = join(appRoot, ".next", "server", "app");

// The retired sample-list route must not come back as a built page.
const retiredRouteDirectories = ["practice/lists"];

// Detail pages render on request. A sibling of `[id]` here is a page that was
// prerendered at build time from a local mock ID.
const onDemandDetailRoutes = ["campus/item", "food/post", "library/item"];

// The sample practice lists must not ship in browser assets or route payloads.
const sampleListMarkers = ["数据结构 · 期末冲刺", "当前页面为示例题单"];

// Mock IDs that used to be prerendered into route payloads.
const mockPrerenderMarkers = ["ds-final", "ml-01"];

for (const root of [staticAssetRoot, serverAppRoot]) {
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

function markersIn(root, markers) {
  const found = [];
  for (const file of filesIn(root)) {
    const contents = readFileSync(file);
    for (const marker of markers) {
      if (contents.includes(Buffer.from(marker))) {
        found.push(`${relative(appRoot, file)} contains ${JSON.stringify(marker)}`);
      }
    }
  }
  return found;
}

const leaked = [];
for (const route of retiredRouteDirectories) {
  const directory = join(serverAppRoot, route);
  if (existsSync(directory)) {
    leaked.push(`${relative(appRoot, directory)} exists`);
  }
}
for (const route of onDemandDetailRoutes) {
  const directory = join(serverAppRoot, route);
  if (!existsSync(directory)) continue;
  for (const entry of readdirSync(directory)) {
    if (entry !== "[id]") {
      leaked.push(`${relative(appRoot, join(directory, entry))} was prerendered`);
    }
  }
}
leaked.push(...markersIn(staticAssetRoot, sampleListMarkers));
leaked.push(
  ...markersIn(serverAppRoot, [...sampleListMarkers, ...mockPrerenderMarkers])
);

if (leaked.length > 0) {
  throw new Error(`Mock content leaked into a production artifact:\n${leaked.join("\n")}`);
}

console.log("Production artifacts contain no mock-prerendered page or sample practice list.");
