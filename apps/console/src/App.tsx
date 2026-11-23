import { useQueryClient } from "@tanstack/react-query";
import { Alert, Button, Modal, Result, Space, Spin, Typography } from "antd";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Route, Routes, useLocation } from "react-router-dom";
import { ConsoleLayout, CONSOLE_NAV_ITEMS } from "@/components/ConsoleLayout";
import {
  ConsoleContext,
  hasConsoleAccess,
  type ConsoleAuthState,
  type ConsoleContextValue,
  type ConsoleOverviewState,
} from "@/lib/console-context";
import {
  consoleLoginHref,
  fetchConsoleOverview,
  fetchConsoleSession,
  logoutConsoleSession,
  type ConsoleOverview,
  type ConsoleSession,
} from "@/lib/console-gateway";
import {
  beginConsolePrincipal,
  isCurrentConsolePrincipal,
  onConsoleSessionExpired,
} from "@/lib/console-session";
import { AccountMembershipsPage } from "@/pages/AccountMembershipsPage";
import { AccountPointsPage } from "@/pages/AccountPointsPage";
import { AccountTicketsPage } from "@/pages/AccountTicketsPage";
import { FoodPage } from "@/pages/FoodPage";
import { LibraryPage } from "@/pages/LibraryPage";
import { CareerPage } from "@/pages/CareerPage";
import { CampusPage } from "@/pages/CampusPage";
import { NoticePage } from "@/pages/NoticePage";
import { OverviewPage } from "@/pages/OverviewPage";
import { PlatformPage } from "@/pages/PlatformPage";
import { PracticeWorkshopPage } from "@/pages/PracticeWorkshopPage";

export function App() {
  const queryClient = useQueryClient();
  const location = useLocation();
  const normalizedPath = location.pathname.replace(/\/+$/, "") || "/";
  const isOverviewPage = normalizedPath === "/";

  const [authState, setAuthState] = useState<ConsoleAuthState>("loading");
  const [principalGeneration, setPrincipalGeneration] = useState(0);
  const [sessionExpired, setSessionExpired] = useState(false);
  const [session, setSession] = useState<ConsoleSession>();
  const [overview, setOverview] = useState<ConsoleOverview>();
  const [overviewState, setOverviewState] = useState<ConsoleOverviewState>("loading");
  const [pendingSignOut, setPendingSignOut] = useState<"signout" | "switch_account" | null>(null);
  const [signingOut, setSigningOut] = useState(false);

  const overviewControllerRef = useRef<AbortController | undefined>(undefined);
  const generationRef = useRef(0);
  const hadAuthenticatedSessionRef = useRef(false);

  const readOverview = useCallback(async () => {
    overviewControllerRef.current?.abort();
    const controller = new AbortController();
    overviewControllerRef.current = controller;
    return new Promise<Awaited<ReturnType<typeof fetchConsoleOverview>>>((resolve) => {
      let settled = false;
      const finish = (result: Awaited<ReturnType<typeof fetchConsoleOverview>>) => {
        if (settled) return;
        settled = true;
        clearTimeout(timer);
        controller.signal.removeEventListener("abort", onAbort);
        resolve(result);
      };
      const onAbort = () => finish({ state: "unavailable" });
      const timer = setTimeout(() => controller.abort(), 3000);
      controller.signal.addEventListener("abort", onAbort, { once: true });
      void fetchConsoleOverview({ signal: controller.signal }).then(finish, () =>
        finish({ state: "unavailable" }),
      );
    });
  }, []);

  const refreshOverview = useCallback(async () => {
    if (!session) return;
    const generation = generationRef.current;
    if (
      !hasConsoleAccess(
        session.access_context.permissions,
        session.access_context.scopes,
        "console.overview.read",
      )
    ) {
      setOverviewState("denied");
      return;
    }
    setOverviewState("loading");
    const result = await readOverview();
    if (!isCurrentConsolePrincipal(generation)) return;
    if (result.state === "authenticated") {
      setOverview(result.overview);
      setOverviewState("ready");
    } else if (result.state === "signed_out") {
      setAuthState("signed_out");
      setSession(undefined);
      setOverview(undefined);
      setOverviewState("unavailable");
    } else if (result.state === "denied") {
      setOverviewState("denied");
    } else {
      setOverviewState("unavailable");
    }
  }, [session, readOverview]);

  const refreshSession = useCallback(async () => {
    overviewControllerRef.current?.abort();
    const generation = beginConsolePrincipal();
    generationRef.current = generation;
    setPrincipalGeneration(generation);
    queryClient.clear();
    setAuthState("loading");
    setSession(undefined);
    setOverview(undefined);

    const result = await fetchConsoleSession();
    if (!isCurrentConsolePrincipal(generation)) return;
    setSessionExpired(false);
    setAuthState(result.state);
    if (result.state !== "authenticated") {
      hadAuthenticatedSessionRef.current = false;
      setSession(undefined);
      setOverview(undefined);
      return;
    }

    hadAuthenticatedSessionRef.current = true;
    setSession(result.session);
    if (
      !hasConsoleAccess(
        result.session.access_context.permissions,
        result.session.access_context.scopes,
        "console.overview.read",
      )
    ) {
      setOverviewState("denied");
      return;
    }

    setOverviewState("loading");
    const overviewRes = await readOverview();
    if (!isCurrentConsolePrincipal(generation)) return;
    if (overviewRes.state === "authenticated") {
      setOverview(overviewRes.overview);
      setOverviewState("ready");
    } else if (overviewRes.state === "signed_out") {
      setAuthState("signed_out");
      setSession(undefined);
      setOverviewState("unavailable");
    } else if (overviewRes.state === "denied") {
      setOverviewState("denied");
    } else {
      setOverviewState("unavailable");
    }
  }, [queryClient, readOverview]);

  useEffect(() => {
    void refreshSession();
    const stop = onConsoleSessionExpired((generation) => {
      const wasAuthenticated = hadAuthenticatedSessionRef.current;
      hadAuthenticatedSessionRef.current = false;
      overviewControllerRef.current?.abort();
      generationRef.current = generation;
      setPrincipalGeneration(generation);
      queryClient.clear();
      setSessionExpired(wasAuthenticated);
      setAuthState("signed_out");
      setSession(undefined);
      setOverview(undefined);
      setPendingSignOut(null);
    });
    return () => {
      overviewControllerRef.current?.abort();
      stop();
      if (isCurrentConsolePrincipal(generationRef.current)) beginConsolePrincipal();
    };
  }, [refreshSession, queryClient]);

  const confirmSignOut = async () => {
    setSigningOut(true);
    hadAuthenticatedSessionRef.current = false;
    overviewControllerRef.current?.abort();
    const generation = beginConsolePrincipal();
    generationRef.current = generation;
    setPrincipalGeneration(generation);
    queryClient.clear();
    setSessionExpired(false);
    try {
      await logoutConsoleSession();
      if (!isCurrentConsolePrincipal(generation)) return;
      setAuthState("signed_out");
      setSession(undefined);
      setOverview(undefined);
    } catch {
      if (isCurrentConsolePrincipal(generation)) setAuthState("unavailable");
    } finally {
      setSigningOut(false);
      setPendingSignOut(null);
    }
  };

  const permissions = session?.access_context.permissions ?? [];
  const scopes = session?.access_context.scopes ?? [];

  const authorizedNav = useMemo(
    () =>
      CONSOLE_NAV_ITEMS.filter((item) => {
        const candidates = item.anyPermissions ?? [item.permission];
        return candidates.some((perm) => hasConsoleAccess(permissions, scopes, perm, item.product));
      }),
    [permissions, scopes],
  );

  const contextValue = useMemo<ConsoleContextValue>(
    () => ({
      authState,
      session,
      operatorId: session?.user.id,
      permissions,
      scopes,
      principalGeneration,
      sessionExpired,
      overview,
      overviewState,
      authorizedNav,
      refreshSession,
      refreshOverview,
      requestSignOut: (kind) => setPendingSignOut(kind),
    }),
    [
      authState,
      session,
      permissions,
      scopes,
      principalGeneration,
      sessionExpired,
      overview,
      overviewState,
      authorizedNav,
      refreshSession,
      refreshOverview,
    ],
  );

  const renderMainBody = () => {
    if (authState === "authenticated") {
      return (
        <div key={principalGeneration}>
          <Routes>
            <Route path="/" element={<OverviewPage />} />
            <Route path="/operations" element={<PlatformPage />} />
            <Route path="/notices" element={<NoticePage />} />
            <Route path="/practice" element={<PracticeWorkshopPage />} />
            <Route path="/library" element={<LibraryPage />} />
            <Route path="/career" element={<CareerPage />} />
            <Route path="/campus" element={<CampusPage />} />
            <Route path="/food" element={<FoodPage />} />
            <Route path="/account" element={<AccountTicketsPage />} />
            <Route path="/account/memberships" element={<AccountMembershipsPage />} />
            <Route path="/account/points" element={<AccountPointsPage />} />
            <Route path="*" element={<OverviewPage />} />
          </Routes>
        </div>
      );
    }

    return (
      <div>
        {isOverviewPage && (
          <div className="console-page-header">
            <h1 id="overview-heading" className="console-page-title">
              工作台总览
            </h1>
          </div>
        )}
        {authState === "loading" && (
          <div
            role="status"
            aria-busy="true"
            style={{
              display: "flex",
              flexDirection: "column",
              alignItems: "center",
              justifyContent: "center",
              minHeight: 280,
              gap: 12,
              color: "#64748b",
            }}
          >
            <Spin size="large" />
            <span>正在验证登录状态…</span>
          </div>
        )}
        {authState === "signed_out" && (
          <Result
            status="info"
            title={sessionExpired ? "登录已过期，请重新登录" : "请先登录管理后台"}
            subTitle={
              sessionExpired
                ? "你的管理员会话已过期，重新登录后将自动回到当前页面。"
                : "登录 HENU Kit 运营账号后即可进入已授权的工作台，登录成功后将回到当前页面。"
            }
            extra={
              <Button type="primary" href={consoleLoginHref()}>
                登录 Console
              </Button>
            }
          />
        )}
        {authState === "denied" && (
          <Result
            status="403"
            title="当前账户没有后台访问权限"
            subTitle="请联系平台管理员为你开通相应模块角色，或切换其他运营账户登录。"
            extra={
              <Button onClick={() => setPendingSignOut("switch_account")}>换个账户登录</Button>
            }
          />
        )}
        {authState === "unavailable" && (
          <Result
            status="error"
            title="暂时无法验证登录状态"
            subTitle="控制台网关暂时不可用，请检查网络连接后重试。"
            extra={
              <Button type="primary" onClick={() => void refreshSession()}>
                重试连接
              </Button>
            }
          />
        )}
      </div>
    );
  };

  return (
    <ConsoleContext.Provider value={contextValue}>
      <ConsoleLayout>{renderMainBody()}</ConsoleLayout>

      <Modal
        open={pendingSignOut !== null}
        title={pendingSignOut === "switch_account" ? "换个账户登录？" : "确认退出登录？"}
        okText={pendingSignOut === "switch_account" ? "换个账户登录" : "确认退出"}
        cancelText="取消"
        confirmLoading={signingOut}
        onOk={() => void confirmSignOut()}
        onCancel={() => setPendingSignOut(null)}
      >
        <Space orientation="vertical" size={8} style={{ marginTop: 8 }}>
          <Typography.Text>
            {pendingSignOut === "switch_account"
              ? "将退出当前账号并返回登录引导，之后可选择其他账号重新登录。"
              : "退出后将结束本次管理会话，当前页面未保存的草稿不会保留。"}
          </Typography.Text>
          {pendingSignOut === "signout" && (
            <Alert type="info" showIcon title="所有已提交并确认的审核与配置记录不受影响。" />
          )}
        </Space>
      </Modal>
    </ConsoleContext.Provider>
  );
}

export default App;
