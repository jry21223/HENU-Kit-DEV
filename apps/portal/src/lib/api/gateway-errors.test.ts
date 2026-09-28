import { readdirSync, readFileSync } from "node:fs";
import { join, resolve } from "node:path";
import { describe, expect, it } from "vitest";
import {
  BINDING_USER_MESSAGE_CODES,
  BINDING_WITHHELD_CODES,
  envelopeUserMessage,
  GATEWAY_USER_MESSAGE_CODES,
  GATEWAY_WITHHELD_CODES,
  gatewayUserMessage,
} from "./gateway-errors";

/**
 * 放行名单要跟着 Gateway 走（#554）：Gateway 新增一个错误码时，要么放进名单，要么在
 * GATEWAY_WITHHELD_CODES 里写明为什么不放行；名单里也不能留着源码里已经没有的码。
 * 源码里用变量传的码（如 OAuth 回调的 code）扫不到，由人写进名单或不放行清单。
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

/** 名单里的码在源码里还以字符串字面量出现（包括先赋给变量再传的码）。 */
function stillWritten(codes: Iterable<string>, sources: string[]) {
  return [...codes].filter((code) => !sources.some((source) => source.includes(`"${code}"`)));
}

// Gateway 自己写出的错误信封：contract.ErrorEnvelope{Error: "…"} 与 writeError(w, r, status, "…", "…")。
const gatewaySources = goSources(join(repoRoot, "services/portal-gateway"));
const gatewayCodes = codesIn(gatewaySources, [
  /ErrorEnvelope\{\s*Error:\s*"([^"]+)"/g,
  /writeError\(\s*w,\s*r,\s*[^,]+,\s*"([^"]+)"/g,
]);

// Gateway 在 /api/v1/account/qq-binding/* 上按 qq-binding 契约原样转发的 Platform Core 绑定错误。
const bindingSources = [readFileSync(join(repoRoot, "services/platform-core/internal/httpapi/qq_bindings.go"), "utf8")];
const bindingCodes = codesIn(bindingSources, [
  /qqFailure\(\s*[^,]+,\s*"([^"]+)"/g,
  /writeError\(\s*w,\s*r,\s*[^,]+,\s*"([^"]+)"/g,
]);

describe("Gateway error code allowlist", () => {
  it("decides on every error code the Gateway writes as a literal", () => {
    expect(gatewayCodes.size).toBeGreaterThan(20);
    expect(
      [...gatewayCodes].filter((code) => !GATEWAY_USER_MESSAGE_CODES.has(code) && !(code in GATEWAY_WITHHELD_CODES))
    ).toEqual([]);
  });

  it("decides on every QQ binding error code Platform Core returns", () => {
    expect(bindingCodes.size).toBeGreaterThan(5);
    expect(
      [...bindingCodes].filter((code) => !BINDING_USER_MESSAGE_CODES.has(code) && !(code in BINDING_WITHHELD_CODES))
    ).toEqual([]);
  });

  it("lists only codes the sources still write", () => {
    expect(stillWritten([...GATEWAY_USER_MESSAGE_CODES, ...Object.keys(GATEWAY_WITHHELD_CODES)], gatewaySources)).toEqual([]);
    expect(stillWritten([...BINDING_USER_MESSAGE_CODES, ...Object.keys(BINDING_WITHHELD_CODES)], bindingSources)).toEqual([]);
  });

  it("never both shows and withholds the same code", () => {
    expect(Object.keys(GATEWAY_WITHHELD_CODES).filter((code) => GATEWAY_USER_MESSAGE_CODES.has(code))).toEqual([]);
    expect(Object.keys(BINDING_WITHHELD_CODES).filter((code) => BINDING_USER_MESSAGE_CODES.has(code))).toEqual([]);
  });
});

describe("gatewayUserMessage", () => {
  it("shows the Gateway's own message for an allowlisted code", () => {
    expect(gatewayUserMessage("practice access denied", " 暂无练习权限。如有疑问，请到账户中心提交工单。 ")).toBe(
      "暂无练习权限。如有疑问，请到账户中心提交工单。"
    );
  });

  it("does not show a message for a withheld code or a code outside the allowlist", () => {
    expect(gatewayUserMessage("DEPENDENCY_UNAVAILABLE", "依赖服务不可用")).toBeNull();
    expect(gatewayUserMessage("not authenticated", "登录已过期，请重新登录")).toBeNull();
    expect(gatewayUserMessage("not found", "内容不存在或已下架")).toBeNull();
    expect(gatewayUserMessage("practice_idempotency_key_invalid", "请求内容不完整，请检查后重试")).toBeNull();
    // 绑定错误码只对 /bind/qq 的嵌套信封生效。
    expect(gatewayUserMessage("LINK_EXPIRED", "绑定链接已失效，请重新发起")).toBeNull();
    expect(gatewayUserMessage(undefined, "资料库暂时无法加载，请稍后重试。")).toBeNull();
  });

  it("does not show an allowlisted code's message unless it is Chinese text", () => {
    expect(gatewayUserMessage("proxy_error", "dial tcp 10.0.0.7:8080: connection refused")).toBeNull();
    expect(gatewayUserMessage("proxy_error", "   ")).toBeNull();
    expect(gatewayUserMessage("proxy_error", undefined)).toBeNull();
    expect(gatewayUserMessage("proxy_error", { text: "服务暂时不可用，请稍后再来" })).toBeNull();
  });
});

describe("envelopeUserMessage", () => {
  it("reads the Gateway's flat envelope and the binding envelope it forwards", () => {
    expect(envelopeUserMessage({ error: "INVALID_REQUEST", message: "请求无效，请重新打开绑定链接" })).toBe(
      "请求无效，请重新打开绑定链接"
    );
    expect(envelopeUserMessage({ error: { code: "ALREADY_BOUND", message: "账号已存在其他绑定，请先解绑" } })).toBe(
      "账号已存在其他绑定，请先解绑"
    );
  });

  it("keeps a nested envelope to the binding codes and a flat one to the Gateway's", () => {
    // Food、Career 透传的嵌套信封也会用 INVALID_REQUEST 以外的码；Gateway 的码只在扁平信封里算数。
    expect(envelopeUserMessage({ error: { code: "proxy_error", message: "服务暂时不可用，请稍后再来" } })).toBeNull();
    expect(envelopeUserMessage({ error: "LINK_EXPIRED", message: "绑定链接已失效，请重新发起" })).toBeNull();
  });

  it("returns null for a withheld code or a body that is not an envelope", () => {
    expect(envelopeUserMessage({ error: { code: "CLIENT_AUTH_FAILED", message: "服务身份验证失败" } })).toBeNull();
    expect(envelopeUserMessage({ error: { code: "NOT_BOUND", message: "请先绑定 HENU KIT 账号" } })).toBeNull();
    expect(envelopeUserMessage({ error: "INVALID_REQUEST" })).toBeNull();
    expect(envelopeUserMessage({ message: "请求无效" })).toBeNull();
    expect(envelopeUserMessage(null)).toBeNull();
    expect(envelopeUserMessage("<html>502</html>")).toBeNull();
  });
});
