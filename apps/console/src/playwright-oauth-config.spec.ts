import { describe, expect, it } from "vitest";

import oauthContinuationConfig from "../playwright.oauth-continuation.config";

describe("OAuth continuation Playwright config", () => {
  it("passes the Console Gateway URL without shell-specific assignment syntax", () => {
    const servers = Array.isArray(oauthContinuationConfig.webServer)
      ? oauthContinuationConfig.webServer
      : [oauthContinuationConfig.webServer];
    const consoleServer = servers.find((server) =>
      server?.command.includes("@henukit/console exec vite")
    );

    expect(consoleServer?.command).not.toMatch(
      /^PLAYWRIGHT_CONSOLE_GATEWAY_URL=/
    );
    expect(consoleServer?.env?.PLAYWRIGHT_CONSOLE_GATEWAY_URL).toBe(
      "http://127.0.0.1:3230"
    );
  });
});
