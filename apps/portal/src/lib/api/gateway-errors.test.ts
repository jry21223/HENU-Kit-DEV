import { readdirSync, readFileSync } from "node:fs";
import { join, resolve } from "node:path";
import { describe, expect, it } from "vitest";
import {
  envelopeUserMessage,
  GATEWAY_USER_MESSAGE_CODES,
  GATEWAY_WITHHELD_CODES,
  gatewayUserMessage,
} from "./gateway-errors";

/**
 * 放行名单要跟着 Gateway 走（#554）：Gateway 新增一个错误码时，要么放进名单，要么在
 * GATEWAY_WITHHELD_CODES 里写明为什么不放行；名单里也不能留着 Gateway 已经不用的码。
 */
const repoRoot = resolve(__dirname, "../../../../..");

function goSources(directory: string): string[] {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = join(directory, entry.name);
    if (entry.isDirectory()) return goSources(path);
    return entry.name.endsWith(".go") && !entry.name.endsWith("_test.go") ? [readFileSync(path, "utf8")] : [];
  });
}

function codesIn(sources: string[], patterns: RegExp[]): Set<string> {
  const codes = new Set<string>();
  for (const source of sources) {
    for (const pattern of patterns) {
      for (const match of source.matchAll(pattern)) codes.add(match[1]);
    }
  }
  return codes;
}

// Gateway 自己写出的错误信封：contract.ErrorEnvelope{Error: "…"} 与 writeError(w, r, status, "…", "…")。
const gatewayCodes = codesIn(goSources(join(repoRoot, "services/portal-gateway")), [
  /ErrorEnvelope\{\s*Error:\s*"([^"]+)"/g,
  /writeError\(\s*w,\s*r,\s*[^,]+,\s*"([^"]+)"/g,
]);

// Gateway 在 /api/v1/account/qq-binding/* 上按 qq-binding 契约原样转发的 Platform Core 绑定错误。
const bindingCodes = codesIn(
  [readFileSync(join(repoRoot, "services/platform-core/internal/httpapi/qq_bindings.go"), "utf8")],
  [/qqFailure\(\s*\d+,\s*"([^"]+)"/g, /writeError\(\s*w,\s*r,\s*\d+,\s*"([^"]+)"/g]
);

describe("Gateway error code allowlist", () => {
  it("decides on every error code the Gateway writes", () => {
    expect(gatewayCodes.size).toBeGreaterThan(20);
    const undecided = [...gatewayCodes, ...bindingCodes].filter(
      (code) => !GATEWAY_USER_MESSAGE_CODES.has(code) && !(code in GATEWAY_WITHHELD_CODES)
    );
    expect(undecided).toEqual([]);
  });

  it("lists only codes the Gateway still writes", () => {
    const stale = [...GATEWAY_USER_MESSAGE_CODES, ...Object.keys(GATEWAY_WITHHELD_CODES)].filter(
      (code) => !gatewayCodes.has(code) && !bindingCodes.has(code)
    );
    expect(stale).toEqual([]);
  });

  it("never both shows and withholds the same code", () => {
    expect(Object.keys(GATEWAY_WITHHELD_CODES).filter((code) => GATEWAY_USER_MESSAGE_CODES.has(code))).toEqual([]);
  });
});

describe("gatewayUserMessage", () => {
  it("shows the Gateway's own message for an allowlisted code", () => {
    expect(gatewayUserMessage("practice access denied", "暂无练习权限，请联系管理员")).toBe("暂无练习权限，请联系管理员");
    expect(gatewayUserMessage("LINK_EXPIRED", " 绑定链接已失效，请重新发起 ")).toBe("绑定链接已失效，请重新发起");
  });

  it("does not show a message for a code outside the allowlist", () => {
    expect(gatewayUserMessage("DEPENDENCY_UNAVAILABLE", "依赖服务不可用")).toBeNull();
    expect(gatewayUserMessage("not authenticated", "登录已过期，请重新登录")).toBeNull();
    expect(gatewayUserMessage(undefined, "资料库暂时无法加载，请稍后重试。")).toBeNull();
  });

  it("does not show an allowlisted code's message unless it is Chinese text", () => {
    expect(gatewayUserMessage("not found", "material not found")).toBeNull();
    expect(gatewayUserMessage("not found", "   ")).toBeNull();
    expect(gatewayUserMessage("not found", undefined)).toBeNull();
    expect(gatewayUserMessage("not found", { text: "内容不存在或已下架" })).toBeNull();
  });
});

describe("envelopeUserMessage", () => {
  it("reads the Gateway's flat envelope and the binding envelope it forwards", () => {
    expect(envelopeUserMessage({ error: "INVALID_REQUEST", message: "请求无效，请重新打开绑定链接" })).toBe(
      "请求无效，请重新打开绑定链接"
    );
    expect(envelopeUserMessage({ error: { code: "ALREADY_BOUND", message: "已绑定账号，请先解绑再换绑" } })).toBe(
      "已绑定账号，请先解绑再换绑"
    );
  });

  it("returns null for a code outside the allowlist or a body that is not an envelope", () => {
    expect(envelopeUserMessage({ error: { code: "CLIENT_AUTH_FAILED", message: "服务身份验证失败" } })).toBeNull();
    expect(envelopeUserMessage({ error: "INVALID_REQUEST" })).toBeNull();
    expect(envelopeUserMessage({ message: "请求无效" })).toBeNull();
    expect(envelopeUserMessage(null)).toBeNull();
    expect(envelopeUserMessage("<html>502</html>")).toBeNull();
  });
});
