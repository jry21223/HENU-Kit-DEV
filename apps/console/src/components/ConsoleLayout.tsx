import {
  BellOutlined,
  BookOutlined,
  CompassOutlined,
  CrownOutlined,
  DashboardOutlined,
  ExperimentOutlined,
  LogoutOutlined,
  MenuOutlined,
  MessageOutlined,
  ReloadOutlined,
  SafetyCertificateOutlined,
  ShopOutlined,
  SwapOutlined,
  WalletOutlined,
} from "@ant-design/icons";
import {
  Breadcrumb,
  Button,
  Drawer,
  Grid,
  Layout,
  Menu,
  Space,
  Tag,
  Typography,
} from "antd";
import type { MenuProps } from "antd";
import { useMemo, useState, type ReactNode } from "react";
import { Link, useLocation, useNavigate } from "react-router-dom";
import {
  ALL_CONSOLE_NAV_ITEMS,
  NAV_GROUP_LABELS,
  useConsole,
  type NavGroupKey,
  type ConsoleNavItem,
} from "@/lib/console-context";
import { consoleLoginHref } from "@/lib/console-gateway";

const { Header, Sider, Content } = Layout;

const ICON_MAP: Record<string, ReactNode> = {
  "/": <DashboardOutlined />,
  "/food": <ShopOutlined />,
  "/notices": <BellOutlined />,
  "/practice": <ExperimentOutlined />,
  "/library": <BookOutlined />,
  "/career": <CompassOutlined />,
  "/campus": <MessageOutlined />,
  "/account": <MessageOutlined />,
  "/account/memberships": <CrownOutlined />,
  "/account/points": <WalletOutlined />,
  "/operations": <SafetyCertificateOutlined />,
};

export const CONSOLE_NAV_ITEMS: ConsoleNavItem[] = [
  ...ALL_CONSOLE_NAV_ITEMS,
  { key: "/career", path: "/career", label: "求职雷达监控", group: "business", permission: "career.read", product: "career", headingId: "career-heading" },
  { key: "/campus", path: "/campus", label: "校园互助治理", group: "business", permission: "campus.read", product: "campus", headingId: "campus-heading" },
];

interface ConsoleLayoutProps {
  children: ReactNode;
}

export function ConsoleLayout({ children }: ConsoleLayoutProps) {
  const {
    authState,
    authorizedNav,
    refreshSession,
    requestSignOut,
  } = useConsole();
  const location = useLocation();
  const navigate = useNavigate();
  const screens = Grid.useBreakpoint();
  const isDesktop = screens.lg ?? true;
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false);

  const normalizedPath = location.pathname.replace(/\/+$/, "") || "/";

  const activeNav = useMemo(
    () =>
      CONSOLE_NAV_ITEMS.find((item) => item.path === normalizedPath) ??
      CONSOLE_NAV_ITEMS[0],
    [normalizedPath],
  );

  const menuItems = useMemo<MenuProps["items"]>(() => {
    const groups: NavGroupKey[] = ["workspace", "business", "account", "platform"];
    const result: NonNullable<MenuProps["items"]> = [];
    for (const group of groups) {
      const items = authorizedNav.filter((item) => item.group === group);
      if (!items.length) continue;
      result.push({
        type: "group",
        key: `group-${group}`,
        label: NAV_GROUP_LABELS[group],
        children: items.map((item) => ({
          key: item.path,
          icon: ICON_MAP[item.path],
          label: item.label,
        })),
      });
    }
    return result;
  }, [authorizedNav]);

  const handleSelect: MenuProps["onClick"] = ({ key }) => {
    setMobileMenuOpen(false);
    if (key !== normalizedPath) {
      navigate(key);
    }
  };

  const brandBlock = (
    <Link
      to="/"
      onClick={() => setMobileMenuOpen(false)}
      style={{
        display: "flex",
        alignItems: "center",
        gap: 10,
        height: 56,
        padding: "0 16px",
        borderBottom: "1px solid #f0f0f0",
        color: "inherit",
        textDecoration: "none",
      }}
    >
      <div
        style={{
          width: 28,
          height: 28,
          borderRadius: 6,
          background: "#18181b",
          color: "#ffffff",
          display: "grid",
          placeItems: "center",
          fontWeight: 700,
          fontSize: 13,
          flexShrink: 0,
        }}
      >
        H
      </div>
      <div style={{ minWidth: 0 }}>
        <div
          style={{
            fontSize: 14,
            fontWeight: 600,
            lineHeight: 1.2,
            color: "#0f172a",
            whiteSpace: "nowrap",
            overflow: "hidden",
            textOverflow: "ellipsis",
          }}
        >
          HENUKit Console
        </div>
        <div style={{ fontSize: 11, color: "#64748b", lineHeight: 1.2 }}>
          运营管理后台
        </div>
      </div>
    </Link>
  );

  const navMenu = (
    <div style={{ display: "flex", flexDirection: "column", height: "100%" }}>
      {brandBlock}
      <div style={{ flex: 1, overflowY: "auto", padding: "8px 6px" }}>
        {menuItems && menuItems.length > 0 ? (
          <Menu
            mode="inline"
            selectedKeys={[normalizedPath]}
            items={menuItems}
            onClick={handleSelect}
            style={{ borderInlineEnd: 0 }}
          />
        ) : (
          <div style={{ padding: "16px 12px", fontSize: 12, color: "#94a3b8" }}>
            {authState === "loading"
              ? "正在加载可用模块…"
              : "登录后显示已授权的运营模块"}
          </div>
        )}
      </div>
      <div className="console-sider-footer">
        <div style={{ marginBottom: 6 }}>
          <a
            href="https://henukit.cn"
            target="_blank"
            rel="noopener noreferrer"
            style={{ color: "#475569", display: "inline-flex", alignItems: "center", gap: 6 }}
          >
            <CompassOutlined />
            打开 HENU Kit 主站
          </a>
        </div>
        <div>学生自主运营 · 非河南大学官方项目</div>
      </div>
    </div>
  );

  const authBadge = (() => {
    if (authState === "loading") {
      return <Tag color="default">正在验证登录…</Tag>;
    }
    if (authState === "authenticated") {
      return (
        <Space size={8}>
          {isDesktop && (
            <Tag color="success" style={{ marginInlineEnd: 0 }}>
              已登录
            </Tag>
          )}
          <Button
            size="small"
            icon={<LogoutOutlined />}
            onClick={() => requestSignOut("signout")}
          >
            退出登录
          </Button>
        </Space>
      );
    }
    if (authState === "denied") {
      return (
        <Space size={8}>
          <Tag color="warning" style={{ marginInlineEnd: 0 }}>
            权限不足
          </Tag>
          <Button
            size="small"
            icon={<SwapOutlined />}
            onClick={() => requestSignOut("switch_account")}
          >
            换个账户登录
          </Button>
        </Space>
      );
    }
    if (authState === "unavailable") {
      return (
        <Button size="small" icon={<ReloadOutlined />} onClick={() => void refreshSession()}>
          重试连接
        </Button>
      );
    }
    return (
      <Button type="primary" size="small" href={consoleLoginHref()}>
        登录 Console
      </Button>
    );
  })();

  return (
    <Layout style={{ minHeight: "100vh" }} data-console-shell>
      {isDesktop ? (
        <Sider
          width={232}
          theme="light"
          style={{
            borderRight: "1px solid #e5e7eb",
            position: "sticky",
            top: 0,
            height: "100vh",
            overflow: "hidden",
          }}
        >
          {navMenu}
        </Sider>
      ) : (
        <Drawer
          placement="left"
          open={mobileMenuOpen}
          onClose={() => setMobileMenuOpen(false)}
          size={256}
          styles={{ body: { padding: 0 } }}
          closable={false}
          title={null}
        >
          {navMenu}
        </Drawer>
      )}

      <Layout style={{ minWidth: 0 }}>
        <Header
          style={{
            position: "sticky",
            top: 0,
            zIndex: 20,
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            gap: 12,
            padding: isDesktop ? "0 20px" : "0 12px",
            borderBottom: "1px solid #e5e7eb",
            background: "rgba(255, 255, 255, 0.92)",
            backdropFilter: "blur(8px)",
          }}
        >
          <div
            style={{
              display: "flex",
              alignItems: "center",
              gap: isDesktop ? 12 : 8,
              minWidth: 0,
              flex: 1,
            }}
          >
            {!isDesktop && (
              <Button
                type="text"
                icon={<MenuOutlined />}
                aria-label="打开运营导航"
                style={{ flexShrink: 0 }}
                onClick={() => setMobileMenuOpen(true)}
              />
            )}
            {isDesktop ? (
              <Breadcrumb
                items={
                  activeNav.path === "/"
                    ? [{ title: "工作台总览" }]
                    : NAV_GROUP_LABELS[activeNav.group] === activeNav.label
                      ? [
                          { title: <Link to="/">工作台总览</Link> },
                          { title: <Typography.Text strong>{activeNav.label}</Typography.Text> },
                        ]
                      : [
                          { title: <Link to="/">工作台总览</Link> },
                          { title: NAV_GROUP_LABELS[activeNav.group] },
                          { title: <Typography.Text strong>{activeNav.label}</Typography.Text> },
                        ]
                }
              />
            ) : (
              <Typography.Text
                strong
                ellipsis
                style={{ fontSize: 15, lineHeight: 1.4, minWidth: 0 }}
              >
                {activeNav.label}
              </Typography.Text>
            )}
          </div>

          <div style={{ display: "flex", alignItems: "center", gap: 8, flexShrink: 0 }}>
            {authBadge}
          </div>
        </Header>

        <Content
          style={{
            width: "100%",
            maxWidth: 1280,
            margin: "0 auto",
            padding: isDesktop ? "24px 28px 40px" : "16px 14px 32px",
          }}
        >
          {children}
        </Content>
      </Layout>
    </Layout>
  );
}
