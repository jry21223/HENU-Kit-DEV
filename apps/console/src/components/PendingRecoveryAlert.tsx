import { Alert, Button } from "antd";

interface PendingRecoveryAlertProps {
  visible: boolean;
  title?: string;
  description: string;
  busy?: boolean;
  disabled?: boolean;
  onResolve: () => void;
  actionLabel?: string;
}

/**
 * 统一未确认操作恢复横幅：
 * 正常操作只用轻量 Toast；只有网络中断或网关超导致写入结果未明时，才优雅露出此横幅，
 * 提示运营点击查询原结果（绝不重复提交）。
 */
export function PendingRecoveryAlert({
  visible,
  title = "有一项操作的结果尚未确认",
  description,
  busy = false,
  disabled = false,
  onResolve,
  actionLabel = "查询处理结果",
}: PendingRecoveryAlertProps) {
  if (!visible) return null;
  return (
    <Alert
      type="warning"
      showIcon
      title={title}
      description={description}
      style={{ marginBottom: 16 }}
      action={
        <Button size="small" loading={busy} disabled={disabled} onClick={onResolve}>
          {actionLabel}
        </Button>
      }
    />
  );
}
