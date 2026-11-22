import { useQuery } from "@tanstack/react-query";
import { Alert, Button, Card, Col, List, Modal, Row, Space, Statistic, Tabs, Tag } from "antd";
import { useRef, useState } from "react";
import { ModuleStateView } from "@/components/ModuleStateView";
import { hasConsoleAccess, useConsole } from "@/lib/console-context";
import { readCareerWorkspace, retryCareerDigest, workspaceWriteFeedback } from "@/lib/console-extensions";
import { localDateTimeSeconds } from "@/lib/format";

const STATUS_LABELS: Record<string, string> = {
  queued: "排队中", running: "进行中", completed: "已完成", failed: "失败",
  sending: "投递中", retry: "等待重试", sent: "已提交投递", skipped: "已跳过",
};
const STAGE_LABELS: Record<string, string> = { crawling: "获取岗位", matching: "匹配岗位", rendering: "整理结果" };

export function CareerPage() {
  const { permissions, scopes, principalGeneration } = useConsole();
  const canRead = hasConsoleAccess(permissions, scopes, "career.read", "career");
  const canManage = hasConsoleAccess(permissions, scopes, "career.manage", "career");
  const query = useQuery({ queryKey: ["career-workspace", principalGeneration], queryFn: ({ signal }) => readCareerWorkspace(signal), enabled: canRead });
  const [target, setTarget] = useState<{ id: string; key: string }>();
  const [busy, setBusy] = useState(false);
  const [failure, setFailure] = useState("");
  const [success, setSuccess] = useState(false);
  const writing = useRef(false);
  const retryKeys = useRef(new Map<string, string>());

  async function retryDigest() {
    if (!target || !canManage || writing.current) return;
    writing.current = true;
    setBusy(true);
    setFailure("");
    try {
      const result = await retryCareerDigest(target.id, target.key);
      if (result.state === "succeeded") {
        retryKeys.current.delete(target.id);
        setTarget(undefined);
        setSuccess(true);
        void query.refetch();
      } else {
        setFailure(workspaceWriteFeedback[result.state]);
      }
    } finally {
      writing.current = false;
      setBusy(false);
    }
  }

  const result = query.data;
  const workspace = canRead && result?.state === "authenticated" ? result.data : undefined;
  const state = !canRead ? "denied" : result?.state === "signed_out" || result?.state === "denied" ? result.state : query.isPending ? "loading" : "unavailable";
  const counts = workspace?.counts;
  return (
    <section aria-labelledby="career-heading" style={{ minWidth: 0 }}>
      <div className="console-page-header">
        <div><h1 id="career-heading" className="console-page-title">求职雷达监控</h1><p className="console-page-subtitle">查看岗位扫描、简历解析和简报投递的进度。</p></div>
        <Button disabled={!canRead} onClick={() => void query.refetch()} loading={query.isFetching}>刷新数据</Button>
      </div>
      {workspace && counts ? <Space orientation="vertical" size="middle" style={{ display: "flex", minWidth: 0 }}>
        {!workspace.job_source_enabled && <Alert type="info" showIcon message="岗位数据源当前暂停，仍可查看历史任务。" />}
        {success && <Alert type="success" showIcon message="已安排重新投递。" closable onClose={() => setSuccess(false)} />}
        <Row gutter={[12, 12]}>
          {[["求职画像", counts.profiles], ["排队扫描", counts.searches_queued], ["进行中扫描", counts.searches_running], ["已完成扫描", counts.searches_completed], ["失败扫描", counts.searches_failed], ["待投递简报", counts.digests_pending]].map(([label, value]) => (
            <Col key={label} xs={12} lg={8} xl={4}><Card size="small"><Statistic title={label} value={value} /></Card></Col>
          ))}
        </Row>
        <Card>
          <Tabs items={[
            { key: "scans", label: "扫描任务监控", children: <List dataSource={workspace.searches} pagination={{ pageSize: 10, hideOnSinglePage: true }} renderItem={(search) => <List.Item><Space wrap>
              <Tag>{STATUS_LABELS[search.status] ?? "未知状态"}</Tag><span>{localDateTimeSeconds(search.created_at)}</span>
              {STAGE_LABELS[search.stage] && <span>{STAGE_LABELS[search.stage]}</span>}
              <span>来源 {search.source_count} · 岗位 {search.job_count} · 匹配 {search.matched_count}</span>
            </Space></List.Item>} /> },
            { key: "extractions", label: "简历解析任务", children: <List dataSource={workspace.extractions} pagination={{ pageSize: 10, hideOnSinglePage: true }} renderItem={(extraction) => <List.Item><Space wrap>
              <Tag>{STATUS_LABELS[extraction.status] ?? "未知状态"}</Tag><span>{localDateTimeSeconds(extraction.created_at)}</span>
              {extraction.completed_at && <span>完成于 {localDateTimeSeconds(extraction.completed_at)}</span>}
            </Space></List.Item>} /> },
            { key: "digests", label: "邮件简报投递", children: <List dataSource={workspace.digest_deliveries} pagination={{ pageSize: 10, hideOnSinglePage: true }} renderItem={(delivery) => <List.Item><Space wrap>
              <Tag>{STATUS_LABELS[delivery.status] ?? "未知状态"}</Tag><span>更新于 {localDateTimeSeconds(delivery.updated_at)}</span>
              {canManage && (delivery.status === "retry" || delivery.status === "failed") && <Button onClick={() => {
                const key = retryKeys.current.get(delivery.search_id) ?? crypto.randomUUID();
                retryKeys.current.set(delivery.search_id, key);
                setFailure("");
                setTarget({ id: delivery.search_id, key });
              }}>重新投递</Button>}
            </Space></List.Item>} /> },
          ]} />
        </Card>
      </Space> : <ModuleStateView state={state} loadingText="正在读取求职雷达任务…" deniedTitle="没有求职雷达查看权限" onRetry={() => void query.refetch()} />}
      {target && <Modal open title="重新投递邮件简报" okText="确认投递" cancelText="取消" confirmLoading={busy} onOk={() => void retryDigest()} onCancel={() => { if (!busy) setTarget(undefined); }}>
        <p>确认安排重新投递这份邮件简报？</p>
        {failure && <Alert type="error" message={failure} />}
      </Modal>}
    </section>
  );
}
