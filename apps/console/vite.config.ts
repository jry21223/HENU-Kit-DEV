import tailwindcss from "@tailwindcss/vite";
import vue from "@vitejs/plugin-vue";
import { defineConfig, loadEnv } from "vite";
import { fileURLToPath, URL } from "node:url";

function normalizeBase(raw: string | undefined): string {
  const value = (raw ?? "/").trim() || "/";
  if (value === "/") return "/";
  const withLeading = value.startsWith("/") ? value : `/${value}`;
  return withLeading.endsWith("/") ? withLeading : `${withLeading}/`;
}

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), "");
  const base = normalizeBase(env.VITE_BASE_PATH ?? process.env.VITE_BASE_PATH);
  const gatewayProxy =
    env.PLAYWRIGHT_CONSOLE_GATEWAY_URL ?? "http://127.0.0.1:8082";

  return {
    base,
    plugins: [vue(), tailwindcss()],
    resolve: {
      alias: {
        "@": fileURLToPath(new URL("./src", import.meta.url)),
      },
    },
    server: {
      host: "0.0.0.0",
      port: 5174,
      proxy: {
        "/api": gatewayProxy,
      },
    },
  };
});
