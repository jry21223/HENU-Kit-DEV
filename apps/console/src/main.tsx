import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { App as AntApp, ConfigProvider } from "antd";
import zhCN from "antd/locale/zh_CN";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import App from "./App";
import { consoleBasePath } from "./lib/base-path";
import "./styles.css";
import { consoleTheme } from "./theme";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: false,
      refetchOnWindowFocus: false,
    },
  },
});

const rootEl = document.getElementById("app");
if (!rootEl) {
  throw new Error("Missing #app root element");
}

createRoot(rootEl).render(
  <StrictMode>
    <ConfigProvider locale={zhCN} theme={consoleTheme}>
      <AntApp>
        <QueryClientProvider client={queryClient}>
          <BrowserRouter basename={consoleBasePath() || "/"}>
            <App />
          </BrowserRouter>
        </QueryClientProvider>
      </AntApp>
    </ConfigProvider>
  </StrictMode>,
);
