import { Button, Result, Spin } from "antd";
import type { ReactNode } from "react";
import { consoleLoginHref } from "@/lib/console-gateway";

interface ModuleStateViewProps {
  state: "loading" | "denied" | "signed_out" | "unavailable" | "not_configured" | "invalid";
  loadingText?: string;
  deniedTitle?: string;
  deniedDescription?: string;
  unavailableTitle?: string;
  unavailableDescription?: string;
  notConfiguredTitle?: string;
  notConfiguredDescription?: string;
  invalidTitle?: string;
  invalidDescription?: string;
  onRetry?: () => void;
  retryLabel?: string;
  extra?: ReactNode;
}

export function ModuleStateView({
  state,
  loadingText = "正在加载数据…",
  deniedTitle = "当前账户没有访问权限",
  deniedDescription = "请联系平台管理员为你开通对应模块权限，或切换其他账户登录。",
  unavailableTitle = "服务暂时不可用",
  unavailableDescription = "暂时无法连接到后台服务，请稍后重试。",
  notConfiguredTitle = "该模块尚未接入",
  notConfiguredDescription = "当前环境未开启该后台能力，不影响学生端正常使用。",
  invalidTitle = "当前页面状态已过期",
  invalidDescription = "列表分页或筛选上下文已失效，请重新加载最新数据。",
  onRetry,
  retryLabel = "重新加载",
  extra,
}: ModuleStateViewProps) {
  if (state === "loading") {
    return (
      <div
        role="status"
        aria-busy="true"
        style={{
          display: "flex",
          flexDirection: "column",
          alignItems: "center",
          justifyContent: "center",
          minHeight: 240,
          gap: 12,
          color: "#64748b",
        }}
      >
        <Spin size="large" />
        <span>{loadingText}</span>
      </div>
    );
  }

  if (state === "signed_out") {
    return (
      <Result
        status="warning"
        title="登录已过期"
        subTitle="请重新登录后继续操作，登录完成后将回到当前页面。"
        extra={
          <Button type="primary" href={consoleLoginHref()}>
            重新登录
          </Button>
        }
      />
    );
  }

  if (state === "denied") {
    return (
      <Result
        status="403"
        title={deniedTitle}
        subTitle={deniedDescription}
        extra={extra}
      />
    );
  }

  if (state === "not_configured") {
    return (
      <Result
        status="info"
        title={notConfiguredTitle}
        subTitle={notConfiguredDescription}
        extra={
          extra ??
          (onRetry ? (
            <Button onClick={onRetry}>重新检查</Button>
          ) : null)
        }
      />
    );
  }

  if (state === "invalid") {
    return (
      <Result
        status="warning"
        title={invalidTitle}
        subTitle={invalidDescription}
        extra={
          onRetry ? (
            <Button type="primary" onClick={onRetry}>
              {retryLabel}
            </Button>
          ) : null
        }
      />
    );
  }

  return (
    <Result
      status="error"
      title={unavailableTitle}
      subTitle={unavailableDescription}
      extra={
        extra ??
        (onRetry ? (
          <Button type="primary" onClick={onRetry}>
            {retryLabel}
          </Button>
        ) : null)
      }
    />
  );
}
