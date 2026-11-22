import { useQuery } from "@tanstack/react-query";
import { Alert, Button, Card, Col, Descriptions, Drawer, Input, List, Modal, Row, Space, Statistic, Tabs, Tag } from "antd";
import { useRef, useState } from "react";
import { ModuleStateView } from "@/components/ModuleStateView";
import { hasConsoleAccess, useConsole } from "@/lib/console-context";
import { moderateCampusItem, moderateCampusMessage, readCampusWorkspace, workspaceWriteFeedback, type CampusModerationItem, type CampusModerationMessage } from "@/lib/console-extensions";
import { localDateTimeSeconds } from "@/lib/format";

const ITEM_STATES = { open: "开放中", ongoing: "进行中", done: "已完成", hidden: "已隐藏" };
const ACTION_LABELS = { hide: "隐藏", restore: "恢复", close: "强制结单" };
type Moderation = { kind: "item"; value: CampusModerationItem; action: "hide" | "restore" | "close" } | { kind: "message"; value: CampusModerationMessage; action: "hide" | "restore" };

export function CampusPage() {
  const { permissions, scopes, principalGeneration } = useConsole();
  const canRead = hasConsoleAccess(permissions, scopes, "campus.read", "campus");
  const canModerate = hasConsoleAccess(permissions, scopes, "campus.moderate", "campus");
  const query = useQuery({ queryKey: ["campus-workspace", principalGeneration], queryFn: ({ signal }) => readCampusWorkspace(signal), enabled: canRead });
  const [selectedID, setSelectedID] = useState<string>();
  const [moderation, setModeration] = useState<Moderation>();
  const [note, setNote] = useState("");
  const [failure, setFailure] = useState("");
  const [success, setSuccess] = useState("");
  const [busy, setBusy] = useState(false);
  const [conflict, setConflict] = useState(false);
  const writing = useRef(false);
  const result = query.data;
  const workspace = canRead && result?.state === "authenticated" ? result.data : undefined;
  const state = !canRead ? "denied" : result?.state === "signed_out" || result?.state === "denied" ? result.state : query.isPending ? "loading" : "unavailable";
  const selected = workspace?.items.find((item) => item.id === selectedID);

  function openModeration(value: Moderation) {
    setNote("");
    setFailure("");
    setSuccess("");
    setConflict(false);
    setModeration(value);
  }
  async function confirmModeration() {
    if (!moderation || !canModerate || writing.current || conflict) return;
    writing.current = true;
    setBusy(true);
    setFailure("");
    const trimmedNote = note.trim();
    try {
      const command = moderation.kind === "item"
        ? await moderateCampusItem(moderation.value.id, { expected_version: moderation.value.version, action: moderation.action, note: trimmedNote || undefined })
        : await moderateCampusMessage(moderation.value.id, { action: moderation.action, note: trimmedNote || undefined });
      if (command.state === "succeeded") {
        setSuccess(`${ACTION_LABELS[moderation.action]}已完成。`);
        setModeration(undefined);
        void query.refetch();
      } else {
        setFailure(workspaceWriteFeedback[command.state]);
        if (command.state === "conflict") { setConflict(true); void query.refetch(); }
      }
    } finally { writing.current = false; setBusy(false); }
  }

  return (
    <section aria-labelledby="campus-heading" style={{ minWidth: 0 }}>
      <div className="console-page-header">
        <div><h1 id="campus-heading" className="console-page-title">校园互助治理</h1><p className="console-page-subtitle">处理公开单子和留言，查看联系方式交换统计。</p></div>
        <Button disabled={!canRead} loading={query.isFetching} onClick={() => void query.refetch()}>刷新数据</Button>
      </div>
      {workspace ? <Space orientation="vertical" size="middle" style={{ display: "flex", minWidth: 0 }}>
        {success && <Alert type="success" message={success} closable onClose={() => setSuccess("")} />}
        <Row gutter={[12, 12]}>
          {[["开放中", workspace.counts.open], ["进行中", workspace.counts.ongoing], ["已完成", workspace.counts.done], ["已隐藏", workspace.counts.hidden], ["交换申请", workspace.counts.exchanges_total], ["双方已同意", workspace.counts.exchanges_accepted]].map(([label, value]) => (
            <Col key={label} xs={12} lg={8} xl={4}><Card size="small"><Statistic title={label} value={value} /></Card></Col>
          ))}
        </Row>
        <Card><Tabs items={[
          { key: "items", label: "互助与闲置治理", children: <List dataSource={workspace.items} pagination={{ pageSize: 10, hideOnSinglePage: true }} renderItem={(item) => <List.Item><div style={{ minWidth: 0, overflowWrap: "anywhere" }}>
            <strong>{item.title}</strong><Space wrap style={{ marginInlineStart: 8 }}><Tag>{item.type === "help" ? "互助" : "闲置"}</Tag><Tag>{ITEM_STATES[item.status] ?? "未知状态"}</Tag><Button onClick={() => setSelectedID(item.id)}>查看单子</Button></Space>
          </div></List.Item>} /> },
          { key: "messages", label: "留言审核", children: <List dataSource={workspace.messages} pagination={{ pageSize: 10, hideOnSinglePage: true }} renderItem={(message) => <List.Item><div style={{ minWidth: 0, overflowWrap: "anywhere" }}>
            {message.item_title && <strong>{message.item_title}</strong>}
            <p>{message.text}</p><Space wrap><Tag>{message.hidden ? "已隐藏" : "公开"}</Tag><span>{localDateTimeSeconds(message.created_at)}</span>
              {canModerate && <Button danger={!message.hidden} onClick={() => openModeration({ kind: "message", action: message.hidden ? "restore" : "hide", value: message })}>{message.hidden ? "恢复留言" : "隐藏留言"}</Button>}
            </Space>
          </div></List.Item>} /> },
        ]} /></Card>
      </Space> : <ModuleStateView state={state} loadingText="正在读取校园互助记录…" deniedTitle="没有校园互助查看权限" onRetry={() => void query.refetch()} />}
      {selected && <Drawer open title="单子详情" onClose={() => setSelectedID(undefined)} styles={{ wrapper: { maxWidth: "100vw" }, body: { overflowWrap: "anywhere" } }}>
        <Descriptions column={1} items={[
          { key: "title", label: "标题", children: selected.title },
          { key: "desc", label: "说明", children: selected.desc },
          { key: "amount", label: "参考金额", children: `¥${selected.price}` },
          { key: "status", label: "状态", children: ITEM_STATES[selected.status] ?? "未知状态" },
          { key: "moderation", label: "治理说明", children: selected.moderation_note || "暂无" },
        ]} />
        {canModerate && <Space wrap>
          <Button danger={selected.status !== "hidden"} onClick={() => openModeration({ kind: "item", value: selected, action: selected.status === "hidden" ? "restore" : "hide" })}>{selected.status === "hidden" ? "恢复单子" : "隐藏单子"}</Button>
          {(selected.status === "open" || selected.status === "ongoing") && <Button danger onClick={() => openModeration({ kind: "item", value: selected, action: "close" })}>强制结单</Button>}
        </Space>}
      </Drawer>}
      {moderation && <Modal open title={ACTION_LABELS[moderation.action] + (moderation.action === "close" ? "" : moderation.kind === "item" ? "单子" : "留言")} okText="确认" cancelText="取消" confirmLoading={busy} okButtonProps={{ disabled: conflict }} onOk={() => void confirmModeration()} onCancel={() => { if (!busy) setModeration(undefined); }}>
        <label>治理说明（选填）<Input.TextArea value={note} maxLength={400} onChange={(event) => setNote(event.target.value)} /></label>
        {failure && <Alert type="error" message={failure} style={{ marginTop: 12 }} />}
      </Modal>}
    </section>
  );
}
