import { CompassOutlined, ReloadOutlined } from "@ant-design/icons";
import { Alert, Button, Card, Descriptions, Drawer, Form, Input, List, Result, Select, Space, Tabs, Tag } from "antd";
import { useCallback, useEffect, useRef, useState } from "react";
import { ModuleStateView } from "@/components/ModuleStateView";
import { hasConsoleAccess, useConsole } from "@/lib/console-context";
import { readCatalogReadiness, readCatalogWorkspace, writeCatalog, workspaceWriteFeedback, type CatalogReadiness, type CatalogCommand, type CatalogCourse, type CatalogMaterial, type CatalogCorrection, type CatalogWorkspace } from "@/lib/console-extensions";
import { localDateTimeSeconds } from "@/lib/format";

export function LibraryPage() {
  const { authState, permissions, scopes } = useConsole();
  const [state, setState] = useState<
    "loading" | "confirmed" | "signed_out" | "denied" | "unavailable"
  >("loading");
  const [readiness, setReadiness] = useState<CatalogReadiness>();
  const seqRef = useRef(0);
  const mountedRef = useRef(true);

  const canRead = hasConsoleAccess(permissions, scopes, "library.read", "library");

  const refresh = useCallback(async () => {
    const seq = ++seqRef.current;
    if (authState !== "authenticated") {
      setState(authState === "loading" ? "loading" : authState);
      return;
    }
    if (!canRead) {
      setState("denied");
      return;
    }
    setState("loading");
    setReadiness(undefined);
    const result = await readCatalogReadiness();
    if (!mountedRef.current || seq !== seqRef.current) return;
    if (result.state === "authenticated") {
      setReadiness(result.data);
      setState("confirmed");
    } else {
      setState(result.state === "not_found" ? "unavailable" : result.state);
    }
  }, [authState, canRead]);

  useEffect(() => {
    mountedRef.current = true;
    void refresh();
    return () => {
      mountedRef.current = false;
    };
  }, [refresh]);

  return (
    <section aria-labelledby="library-heading">
      <div className="console-page-header">
        <div>
          <h1 id="library-heading" className="console-page-title">
            资料库管理
          </h1>
          <p className="console-page-subtitle">
            查看资料管理服务接入状态，或直接访问学生端公开资料目录。
          </p>
        </div>
        <Button icon={<ReloadOutlined />} loading={state === "loading"} onClick={() => void refresh()}>
          重新检查
        </Button>
      </div>

      {state !== "confirmed" ? (
        <ModuleStateView
          state={state}
          loadingText="正在检查资料库管理服务状态…"
          deniedTitle="没有资料库查看权限"
          onRetry={() => void refresh()}
        />
      ) : readiness?.management === "ready" ? (
        <LibraryCatalog readiness={readiness} />
      ) : readiness ? (
        <Card>
          <Result
            status="info"
            title={
              readiness.management === "not_ready"
                ? "资料在线管理功能尚未就绪"
                : "资料目录暂不可用"
            }
            subTitle={
              readiness.management === "not_ready"
                ? "资料库后台目前处于只读发布模式，课程与资料上架通过离线审核流水线完成；公开资料目录正常运行。"
                : "请稍后重新检查。"
            }
            extra={
              <Button
                type="primary"
                icon={<CompassOutlined />}
                href="https://henukit.cn/library"
                target="_blank"
                rel="noopener noreferrer"
              >
                打开公开资料目录
              </Button>
            }
          />
          <Descriptions bordered size="small" column={{ xs: 1, sm: 2 }}>
            <Descriptions.Item label="管理接口状态">
              <Tag color="warning">尚未就绪</Tag>
            </Descriptions.Item>
            <Descriptions.Item label="检查时间">
              {localDateTimeSeconds(readiness.checked_at)}
            </Descriptions.Item>
          </Descriptions>
        </Card>
      ) : null}
    </section>
  );
}

const MATERIAL_TYPES = { handout: "复习讲义", exam: "往年真题", slides: "课件", exercise: "题库练习", answer: "答案解析", note: "笔记总结", textbook: "电子版教材" };
const ACCESS_LEVELS = { public_free: "公开免费", authenticated: "登录可见", restricted: "限制访问" };
const MATERIAL_STATUS = { draft: "草稿", pending_review: "待审核", published: "已发布", archived: "已归档" };
const COURSE_STATUS = { active: "使用中", archived: "已归档" };
const CORRECTION_STATUS = { open: "待处理", in_progress: "处理中", resolved: "已解决", dismissed: "已驳回" };
const options = (labels: Record<string, string>) => Object.entries(labels).map(([value, label]) => ({ value, label }));
type Editor =
  | { kind: "course"; value?: CatalogCourse }
  | { kind: "material"; value?: CatalogMaterial }
  | { kind: "approve" | "reject"; value: CatalogMaterial }
  | { kind: "correction"; value: CatalogCorrection };

function LibraryCatalog({ readiness }: { readiness: CatalogReadiness }) {
  const { permissions, scopes } = useConsole();
  const [workspace, setWorkspace] = useState<CatalogWorkspace>();
  const [failure, setFailure] = useState<"denied" | "signed_out" | "unavailable">();
  const [revision, setRevision] = useState(0);
  const [editor, setEditor] = useState<Editor>();
  const capabilities = readiness.capabilities;
  const manage = capabilities.commands && hasConsoleAccess(permissions, scopes, "library.manage", "library");
  const review = capabilities.commands && hasConsoleAccess(permissions, scopes, "library.review", "library");
  useEffect(() => {
    const controller = new AbortController();
    void readCatalogWorkspace(controller.signal).then((result) => {
      if (controller.signal.aborted) return;
      if (result.state === "authenticated") {
        setWorkspace(result.data);
        setFailure(undefined);
      } else {
        setWorkspace(undefined);
        setFailure(result.state === "not_found" ? "unavailable" : result.state);
      }
    });
    return () => controller.abort();
  }, [revision]);
  const refresh = () => setRevision((value) => value + 1);
  if (!workspace) return <ModuleStateView state={failure ?? "loading"} loadingText="正在加载资料目录…" onRetry={refresh} />;

  const courseWrite = manage && capabilities.courses_write !== false;
  const materialWrite = manage && capabilities.materials_write !== false;
  const canReview = review && capabilities.review !== false;
  const submissions = new Set(workspace.submissions.map((item) => item.id));
  const materials = [...workspace.materials, ...workspace.submissions.filter((item) => !workspace.materials.some((material) => material.id === item.id))];
  const items = [
    { key: "courses", label: "课程目录", children: <>
      {courseWrite && <Button onClick={() => setEditor({ kind: "course" })}>新建课程</Button>}
      <List dataSource={workspace.courses} pagination={{ pageSize: 10, hideOnSinglePage: true }} renderItem={(course) => (
        <List.Item><Space wrap>
          <strong className="[overflow-wrap:anywhere]">{course.name}</strong><span>{course.code}</span><span>{course.department}</span>
          <Tag>{COURSE_STATUS[course.status] ?? "未知状态"}</Tag>
          {courseWrite && <Button onClick={() => setEditor({ kind: "course", value: course })}>编辑课程</Button>}
        </Space></List.Item>
      )} />
    </> },
    { key: "materials", label: "资料与审核", children: <>
      {materialWrite && <Button disabled={!workspace.courses.length} onClick={() => setEditor({ kind: "material" })}>新建资料</Button>}
      <List dataSource={materials} pagination={{ pageSize: 10, hideOnSinglePage: true }} renderItem={(material) => (
        <List.Item><div style={{ minWidth: 0, overflowWrap: "anywhere" }}>
          <strong>{material.title}</strong>
          <p>{material.course_name || workspace.courses.find((course) => course.id === material.course_id)?.name || "未显示课程"}</p>
          <Space wrap>
            <Tag>{MATERIAL_STATUS[material.status] ?? "未知状态"}</Tag>
            {materialWrite && <Button onClick={() => setEditor({ kind: "material", value: material })}>编辑资料</Button>}
            {canReview && (material.status === "pending_review" || submissions.has(material.id)) && <>
              <Button onClick={() => setEditor({ kind: "approve", value: material })}>通过</Button>
              <Button danger onClick={() => setEditor({ kind: "reject", value: material })}>驳回</Button>
            </>}
          </Space>
        </div></List.Item>
      )} />
    </> },
    { key: "corrections", label: "纠错处理", children: <List dataSource={workspace.corrections} pagination={{ pageSize: 10, hideOnSinglePage: true }} renderItem={(correction) => (
      <List.Item><div style={{ minWidth: 0, overflowWrap: "anywhere" }}>
        <strong>{correction.title || correction.material_title || "资料纠错"}</strong><p>{correction.description}</p>
        <Tag>{CORRECTION_STATUS[correction.status] ?? "未知状态"}</Tag>
        {canReview && <Button onClick={() => setEditor({ kind: "correction", value: correction })}>处理纠错</Button>}
      </div></List.Item>
    )} /> },
    { key: "releases", label: "公开版本与下载统计", children: <>
      {workspace.download_stats && <Descriptions column={{ xs: 1, sm: 2 }} items={[
        { key: "total", label: "累计下载次数", children: workspace.download_stats.total_starts },
        { key: "recent", label: "近 24 小时下载次数", children: workspace.download_stats.last_24h_starts },
      ]} />}
      <List dataSource={workspace.releases ?? []} pagination={{ pageSize: 10, hideOnSinglePage: true }} renderItem={(release) => (
        <List.Item>
          <div style={{ minWidth: 0, overflowWrap: "anywhere" }}>
            <Space wrap>
              <Tag>{release.state === "active" ? "当前版本" : "历史版本"}</Tag>
              <span title={release.release_id}>
                {release.release_id.length > 18 ? `${release.release_id.slice(0, 8)}…${release.release_id.slice(-6)}` : release.release_id}
              </span>
            </Space>
            <p>{localDateTimeSeconds(release.activated_at)} · {release.snapshot_count} 份资料</p>
          </div>
        </List.Item>
      )} />
    </> },
  ];
  return (
    <Card>
      {workspace.degraded && <Alert type="warning" message="暂时无法读取完整资料目录，请稍后重新检查。" />}
      <Tabs items={items} />
      {editor && <CatalogEditor editor={editor} courses={workspace.courses} onClose={() => setEditor(undefined)} onSaved={() => { setEditor(undefined); refresh(); }} onConflict={refresh} />}
    </Card>
  );
}

function CatalogEditor({ editor, courses, onClose, onSaved, onConflict }: { editor: Editor; courses: CatalogCourse[]; onClose: () => void; onSaved: () => void; onConflict: () => void }) {
  const [busy, setBusy] = useState(false);
  const [failure, setFailure] = useState("");
  const [conflicted, setConflicted] = useState(false);
  const replay = useRef<{ body: string; key: string } | undefined>(undefined);
  const active = useRef(false);
  const course = editor.kind === "course";
  const material = editor.kind === "material";
  const correction = editor.kind === "correction";
  const title = course ? (editor.value ? "编辑课程" : "新建课程") : material ? (editor.value ? "编辑资料" : "新建资料") : correction ? "处理纠错" : editor.kind === "approve" ? "通过资料审核" : "驳回资料审核";
  const initial = editor.kind === "material" ? { access_level: "public_free", ...editor.value, material_type: editor.value?.type } : editor.kind === "correction" ? { status: "resolved", resolution_note: editor.value.resolution_note } : editor.value;
  async function save(values: Record<string, string>) {
    if (active.current || conflicted) return;
    active.current = true;
    const payload = { ...values };
    const kind = course ? editor.value ? "course_update" : "course_create" : material ? editor.value ? "material_update" : "material_create" : correction ? "correction_resolve" : editor.kind === "approve" ? "submission_approve" : "submission_reject";
    const command: CatalogCommand = { kind, payload, resource_id: editor.value?.id, expected_version: editor.value ? String(editor.value.version) : undefined };
    const body = JSON.stringify(command);
    if (replay.current?.body !== body) replay.current = { body, key: crypto.randomUUID() };
    setBusy(true);
    setFailure("");
    try {
      const result = await writeCatalog(command, replay.current.key);
      if (result.state === "succeeded") onSaved();
      else {
        setFailure(workspaceWriteFeedback[result.state]);
        if (result.state === "conflict") { setConflicted(true); onConflict(); }
      }
    } finally { active.current = false; setBusy(false); }
  }
  return (
    <Drawer open title={title} onClose={busy ? undefined : onClose} size="default" styles={{ wrapper: { maxWidth: "100vw" } }}>
      <Form layout="vertical" initialValues={initial} onFinish={(values) => void save(values)}>
        {course && <>
          {!editor.value && <Form.Item name="code" label="课程编号" rules={[{ required: true, whitespace: true }]}><Input maxLength={80} /></Form.Item>}
          <Form.Item name="name" label="课程名称" rules={[{ required: true, whitespace: true }]}><Input maxLength={200} /></Form.Item>
          <Form.Item name="department" label="院系"><Input maxLength={120} /></Form.Item>
          {editor.value && <Form.Item name="status" label="课程状态"><Select options={options(COURSE_STATUS)} /></Form.Item>}
        </>}
        {material && <>
          {!editor.value && <Form.Item name="course_id" label="所属课程" rules={[{ required: true }]}><Select options={courses.filter((entry) => entry.status === "active").map((entry) => ({ label: entry.name, value: entry.id }))} /></Form.Item>}
          <Form.Item name="title" label="资料标题" rules={[{ required: true, whitespace: true }]}><Input maxLength={200} /></Form.Item>
          <Form.Item name="material_type" label="资料类型" rules={[{ required: true }]}><Select options={options(MATERIAL_TYPES)} /></Form.Item>
          <Form.Item name="access_level" label="访问范围"><Select options={options(ACCESS_LEVELS)} /></Form.Item>
          {editor.value && <Form.Item name="status" label="资料状态"><Select options={options(MATERIAL_STATUS)} /></Form.Item>}
          <Form.Item name="description" label="资料说明"><Input.TextArea maxLength={2000} /></Form.Item>
          <Form.Item name="source_note" label="来源说明"><Input.TextArea maxLength={1000} /></Form.Item>
        </>}
        {correction && <>
          <Form.Item name="status" label="处理结果" rules={[{ required: true }]}><Select options={options({ in_progress: "处理中", resolved: "已解决", dismissed: "已驳回" })} /></Form.Item>
          <Form.Item name="resolution_note" label="处理说明"><Input.TextArea maxLength={1000} /></Form.Item>
        </>}
        {!course && !material && !correction && <><p>资料：{editor.value.title}</p><Form.Item name="note" label="审核说明"><Input.TextArea maxLength={1000} /></Form.Item></>}
        {failure && <Alert type="error" message={failure} style={{ marginBottom: 12 }} />}
        <Button type="primary" htmlType="submit" loading={busy} disabled={conflicted}>{course || material ? "保存" : "确认处理"}</Button>
      </Form>
    </Drawer>
  );
}
