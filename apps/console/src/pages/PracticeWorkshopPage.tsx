import {
  CheckCircleOutlined,
  CloudUploadOutlined,
  EyeOutlined,
  PlusOutlined,
  ReloadOutlined,
  RollbackOutlined,
  SearchOutlined,
  StopOutlined,
  UploadOutlined,
} from "@ant-design/icons";
import {
  Alert,
  App as AntApp,
  Button,
  Card,
  Descriptions,
  Drawer,
  Empty,
  Form,
  Input,
  Modal,
  Space,
  Table,
  Tag,
  Typography,
  Upload,
} from "antd";
import type { ColumnsType } from "antd/es/table";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ModuleStateView } from "@/components/ModuleStateView";
import { useConsole } from "@/lib/console-context";
import { localDateTime } from "@/lib/format";
import {
  createWorkshopBank,
  fetchWorkshopCatalog,
  fetchWorkshopVersionDetail,
  importWorkshopBankVersion,
  publishWorkshopBankVersion,
  rollbackWorkshopBank,
  sha256Hex,
  unpublishWorkshopBankVersion,
  validateWorkshopBankVersion,
  type WorkshopBankVersionSummary,
  type WorkshopCatalogBank,
  type WorkshopImportReport,
  type WorkshopQuestionItem,
  type WorkshopVersionDetail,
} from "@/lib/quizcraft-workshop";

function versionStateTag(state: string) {
  switch (state) {
    case "published":
      return <Tag color="success">已上架</Tag>;
    case "validated":
      return <Tag color="blue">已校验</Tag>;
    case "draft":
      return <Tag color="processing">草稿</Tag>;
    case "unpublished":
      return <Tag color="default">已下架</Tag>;
    case "legacy":
      return <Tag color="default">历史版本</Tag>;
    default:
      return <Tag>{state}</Tag>;
  }
}

function questionTypeLabel(type: string): string {
  return (
    {
      single: "单选",
      multi: "多选",
      multiple: "多选",
      judge: "判断",
      blank: "填空",
    }[type] ?? type
  );
}

function renderFormattedStem(text: string) {
  const parts = text.split(/(```[\s\S]*?```)/g);
  return parts.map((part, idx) => {
    if (part.startsWith("```") && part.endsWith("```")) {
      const inner = part.slice(3, -3).replace(/^[a-zA-Z0-9_-]*\n/, "");
      return (
        <pre
          key={idx}
          style={{
            margin: "6px 0",
            padding: "8px 10px",
            background: "#f8fafc",
            border: "1px solid #e2e8f0",
            borderRadius: 6,
            overflowX: "auto",
            fontSize: 12,
            lineHeight: 1.55,
          }}
          className="console-mono"
        >
          <code>{inner}</code>
        </pre>
      );
    }
    const inlineParts = part.split(/(`[^`]+`)/g);
    return (
      <span key={idx} style={{ whiteSpace: "pre-wrap" }}>
        {inlineParts.map((seg, sIdx) =>
          seg.startsWith("`") && seg.endsWith("`") ? (
            <code key={sIdx} className="console-code-chip" style={{ margin: "0 2px" }}>
              {seg.slice(1, -1)}
            </code>
          ) : (
            seg
          ),
        )}
      </span>
    );
  });
}

function formatOptionIndex(val: unknown): string {
  if (typeof val === "number" && Number.isInteger(val) && val >= 0 && val < 26) {
    return String.fromCharCode(65 + val);
  }
  return String(val);
}

function formatQuestionAnswer(question: WorkshopQuestionItem): string {
  const { answer, type } = question;
  const isChoice = type === "single" || type === "multi" || type === "multiple";
  if (typeof answer === "boolean") {
    return answer ? "正确" : "错误";
  }
  if (type === "judge" && typeof answer === "string") {
    const normalized = answer.trim().toLowerCase();
    if (normalized === "true") return "正确";
    if (normalized === "false") return "错误";
  }
  if (Array.isArray(answer)) {
    return answer
      .map((item) => {
        if (typeof item === "boolean") return item ? "正确" : "错误";
        if (isChoice) return formatOptionIndex(item);
        return String(item);
      })
      .join("、");
  }
  if (isChoice && typeof answer === "number") {
    return formatOptionIndex(answer);
  }
  if (typeof answer === "string") {
    return answer;
  }
  return JSON.stringify(answer);
}

export function PracticeWorkshopPage() {
  const { message } = AntApp.useApp();
  const { authState, permissions } = useConsole();

  const [state, setState] = useState<
    "loading" | "ready" | "denied" | "disabled" | "signed_out" | "unavailable"
  >("loading");
  const [banks, setBanks] = useState<WorkshopCatalogBank[]>([]);
  const [keyword, setKeyword] = useState("");
  const [busy, setBusy] = useState(false);

  // 新建题库弹窗
  const [createOpen, setCreateOpen] = useState(false);
  const [createForm] = Form.useForm();

  // 导入草稿版本抽屉
  const [importBank, setImportBank] = useState<WorkshopCatalogBank | null>(null);
  const [importFileName, setImportFileName] = useState("");
  const [importJson, setImportJson] = useState("");
  const [importNotes, setImportNotes] = useState("");
  const [importReport, setImportReport] = useState<WorkshopImportReport | null>(null);

  // 版本题目预览抽屉
  const [previewTarget, setPreviewTarget] = useState<{
    bank: WorkshopCatalogBank;
    version: WorkshopBankVersionSummary;
  } | null>(null);
  const [previewLoading, setPreviewLoading] = useState(false);
  const [previewDetail, setPreviewDetail] = useState<WorkshopVersionDetail | null>(null);

  // 生命周期高风险操作确认弹窗
  const [lifecycleModal, setLifecycleModal] = useState<{
    action: "validate" | "publish" | "unpublish" | "rollback";
    bank: WorkshopCatalogBank;
    version: WorkshopBankVersionSummary;
  } | null>(null);
  const [lifecycleNote, setLifecycleNote] = useState("");

  const mountedRef = useRef(true);
  const seqRef = useRef(0);

  const canWrite = permissions.includes("quizcraft.workshop.write");
  const canPublish = permissions.includes("quizcraft.workshop.publish");

  const refresh = useCallback(async () => {
    if (authState !== "authenticated") {
      setState(authState === "denied" ? "denied" : "unavailable");
      return;
    }
    const seq = ++seqRef.current;
    setState("loading");
    const res = await fetchWorkshopCatalog();
    if (!mountedRef.current || seq !== seqRef.current) return;
    if (res.state === "authenticated") {
      setBanks(res.data);
      setState("ready");
    } else if (res.state === "disabled") {
      setState("disabled");
    } else if (res.state === "denied") {
      setState("denied");
    } else if (res.state === "signed_out") {
      setState("signed_out");
    } else {
      setState("unavailable");
    }
  }, [authState]);

  useEffect(() => {
    mountedRef.current = true;
    void refresh();
    return () => {
      mountedRef.current = false;
    };
  }, [refresh]);

  const handleCreateBank = async () => {
    const values = await createForm.validateFields();
    setBusy(true);
    const key = `idem_quiz_bank_create_${crypto.randomUUID()}`;
    const res = await createWorkshopBank(
      {
        name: String(values.name).trim(),
        slug: String(values.slug).trim(),
        subject: values.subject ? String(values.subject).trim() : undefined,
        description: values.description ? String(values.description).trim() : undefined,
      },
      key,
    );
    if (!mountedRef.current) return;
    setBusy(false);
    if (res.state === "succeeded") {
      message.success("题库创建成功。");
      createForm.resetFields();
      setCreateOpen(false);
      await refresh();
    } else {
      message.error(res.state === "conflict" ? "该题库标识已存在，请换一个。" : "创建题库失败，请检查输入后重试。");
    }
  };

  const handleImportFile = async (file: File) => {
    const text = await file.text();
    setImportFileName(file.name);
    setImportJson(text);
    setImportReport(null);
  };

  const handleImportSubmit = async () => {
    if (!importBank) return;
    let parsed: unknown;
    try {
      parsed = JSON.parse(importJson);
    } catch {
      message.error("JSON 格式有误，请检查括号与引号后重试。");
      return;
    }
    const questions: WorkshopQuestionItem[] | undefined = Array.isArray(parsed)
      ? parsed
      : parsed && typeof parsed === "object" && Array.isArray((parsed as { questions?: unknown }).questions)
        ? ((parsed as { questions: WorkshopQuestionItem[] }).questions)
        : undefined;
    if (!questions || questions.length === 0) {
      message.error("JSON 中未找到非空的 questions 题目数组。");
      return;
    }
    setBusy(true);
    setImportReport(null);
    const digest = await sha256Hex(importJson);
    const key = `idem_quiz_import_${crypto.randomUUID()}`;
    const res = await importWorkshopBankVersion(
      importBank.id,
      {
        expected_version: importBank.version,
        source_sha256: digest,
        questions,
        notes: importNotes.trim() || undefined,
      },
      key,
    );
    if (!mountedRef.current) return;
    setBusy(false);
    if (res.state === "succeeded") {
      message.success("新版本草稿导入并校验通过。");
      setImportBank(null);
      setImportFileName("");
      setImportJson("");
      setImportNotes("");
      await refresh();
    } else if (res.state === "validation_failed") {
      setImportReport(res.report);
      message.warning("题库校验未通过，未写入任何数据；请根据下方明细修正题目。");
    } else if (res.state === "conflict") {
      message.warning("该题库刚刚发生变更，请刷新目录后重试。");
      await refresh();
    } else {
      message.error("导入失败，请稍后重试。");
    }
  };

  const openVersionPreview = async (
    bank: WorkshopCatalogBank,
    version: WorkshopBankVersionSummary,
  ) => {
    setPreviewTarget({ bank, version });
    setPreviewDetail(null);
    setPreviewLoading(true);
    const res = await fetchWorkshopVersionDetail(bank.id, version.id);
    if (!mountedRef.current) return;
    setPreviewLoading(false);
    if (res.state === "authenticated") {
      setPreviewDetail(res.data);
    } else {
      message.error("暂时无法加载该版本的完整题目。");
    }
  };

  const handleLifecycleSubmit = async () => {
    if (!lifecycleModal) return;
    const { action, bank, version } = lifecycleModal;
    setBusy(true);
    const key = `idem_quiz_${action}_${crypto.randomUUID()}`;
    const note = lifecycleNote.trim() || undefined;
    const res =
      action === "validate"
        ? await validateWorkshopBankVersion(bank.id, version.id, { expected_version: bank.version, note }, key)
        : action === "publish"
          ? await publishWorkshopBankVersion(bank.id, version.id, { expected_version: bank.version, note }, key)
          : action === "unpublish"
            ? await unpublishWorkshopBankVersion(bank.id, version.id, { expected_version: bank.version, note }, key)
            : await rollbackWorkshopBank(
                bank.id,
                { expected_version: bank.version, target_version_id: version.id, note },
                key,
              );
    if (!mountedRef.current) return;
    setBusy(false);
    if (res.state === "succeeded") {
      message.success(
        {
          validate: "已记录人工校验通过，现在可以发布上架。",
          publish: "该版本已正式发布上架。",
          unpublish: "该版本已下架（历史作答记录仍保留）。",
          rollback: "已成功回滚至所选历史版本。",
        }[action],
      );
      setLifecycleModal(null);
      setLifecycleNote("");
      await refresh();
    } else if (res.state === "conflict") {
      message.warning("题库版本已变化，已为你刷新最新列表。");
      setLifecycleModal(null);
      await refresh();
    } else {
      message.error(res.state === "denied" ? "当前账户缺少执行该操作的权限。" : "操作未能完成，请稍后重试。");
    }
  };

  const filteredBanks = useMemo(() => {
    const q = keyword.trim().toLocaleLowerCase();
    return banks.filter(
      (b) =>
        !q ||
        b.name.toLocaleLowerCase().includes(q) ||
        (b.slug ?? "").toLocaleLowerCase().includes(q) ||
        (b.subject ?? "").toLocaleLowerCase().includes(q),
    );
  }, [banks, keyword]);

  const bankColumns: ColumnsType<WorkshopCatalogBank> = [
    {
      title: "题库名称",
      key: "name",
      render: (_, bank) => (
        <div>
          <Typography.Text strong>{bank.name}</Typography.Text>
          {bank.slug && (
            <span className="console-code-chip" style={{ marginLeft: 8 }}>
              {bank.slug}
            </span>
          )}
          {bank.description && (
            <div style={{ marginTop: 2, fontSize: 12, color: "#64748b" }}>{bank.description}</div>
          )}
        </div>
      ),
    },
    {
      title: "线上版本",
      key: "active",
      width: 140,
      render: (_, bank) =>
        bank.active_version_number ? (
          <Tag color="success">v{bank.active_version_number} 已上架</Tag>
        ) : (
          <Tag>未上架</Tag>
        ),
    },
    {
      title: "版本数",
      key: "versions",
      width: 90,
      render: (_, bank) => bank.versions?.length ?? 0,
    },
    {
      title: "更新时间",
      dataIndex: "updated_at",
      width: 160,
      render: (val?: string) => localDateTime(val),
    },
    {
      title: "操作",
      key: "actions",
      width: 150,
      render: (_, bank) =>
        canWrite ? (
          <Button
            size="small"
            icon={<UploadOutlined />}
            onClick={() => {
              setImportBank(bank);
              setImportFileName("");
              setImportJson("");
              setImportNotes("");
              setImportReport(null);
            }}
          >
            导入新版本
          </Button>
        ) : (
          <Typography.Text type="secondary">仅查看</Typography.Text>
        ),
    },
  ];

  const renderVersionTable = (bank: WorkshopCatalogBank) => {
    const versions = bank.versions ?? [];
    const cols: ColumnsType<WorkshopBankVersionSummary> = [
      {
        title: "版本号",
        dataIndex: "version_number",
        width: 90,
        render: (n: number) => <Typography.Text strong>v{n}</Typography.Text>,
      },
      {
        title: "状态",
        dataIndex: "state",
        width: 110,
        render: (s: string) => versionStateTag(s),
      },
      {
        title: "题量",
        dataIndex: "question_count",
        width: 90,
        render: (c?: number) => (typeof c === "number" ? `${c} 题` : "—"),
      },
      {
        title: "创建时间",
        dataIndex: "created_at",
        width: 160,
        render: (v?: string) => localDateTime(v),
      },
      {
        title: "生命周期操作",
        key: "ops",
        render: (_, ver) => (
          <Space size={6} wrap>
            <Button
              size="small"
              icon={<EyeOutlined />}
              onClick={() => void openVersionPreview(bank, ver)}
            >
              审阅题目
            </Button>
            {canWrite && ver.state === "draft" && (
              <Button
                size="small"
                icon={<CheckCircleOutlined />}
                onClick={() => {
                  setLifecycleNote("");
                  setLifecycleModal({ action: "validate", bank, version: ver });
                }}
              >
                通过校验
              </Button>
            )}
            {canPublish && ver.state === "validated" && (
              <Button
                type="primary"
                size="small"
                icon={<CloudUploadOutlined />}
                onClick={() => {
                  setLifecycleNote("");
                  setLifecycleModal({ action: "publish", bank, version: ver });
                }}
              >
                发布上架
              </Button>
            )}
            {canPublish && ver.state === "published" && (
              <Button
                danger
                size="small"
                icon={<StopOutlined />}
                onClick={() => {
                  setLifecycleNote("");
                  setLifecycleModal({ action: "unpublish", bank, version: ver });
                }}
              >
                下架
              </Button>
            )}
            {canPublish && ver.state === "unpublished" && ver.validated_at && (
              <Button
                size="small"
                icon={<RollbackOutlined />}
                onClick={() => {
                  setLifecycleNote("");
                  setLifecycleModal({ action: "rollback", bank, version: ver });
                }}
              >
                回滚至此版本
              </Button>
            )}
          </Space>
        ),
      },
    ];
    return (
      <Table
        rowKey="id"
        size="small"
        columns={cols}
        dataSource={versions}
        scroll={{ x: "max-content" }}
        pagination={false}
        locale={{ emptyText: "该题库尚未导入版本，请点击「导入新版本」上传题库" }}
      />
    );
  };

  return (
    <section aria-labelledby="practice-heading">
      <div className="console-page-header">
        <div>
          <h1 id="practice-heading" className="console-page-title">
            题库管理
          </h1>
          <p className="console-page-subtitle">
            统一管理刷题题库的创建、JSON 导入校验、人工逐题抽查、发布上架、下架与历史版本回滚。
          </p>
        </div>
        <Space wrap>
          {canWrite && state === "ready" && (
            <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateOpen(true)}>
              新建题库
            </Button>
          )}
          <Button icon={<ReloadOutlined />} loading={state === "loading"} onClick={() => void refresh()}>
            刷新
          </Button>
        </Space>
      </div>

      {state !== "ready" ? (
        <ModuleStateView
          state={state === "disabled" ? "not_configured" : state}
          loadingText="正在读取题库目录…"
          deniedTitle="没有题库管理访问权限"
          notConfiguredTitle="题库管理后台尚未开启"
          notConfiguredDescription="当前环境尚未开启题库管理桥接，请联系平台管理员在部署配置中启用；开启前不影响学生端正常刷题。"
          onRetry={() => void refresh()}
        />
      ) : (
        <Card>
          <div style={{ marginBottom: 14 }}>
            <Input
              allowClear
              prefix={<SearchOutlined />}
              placeholder="按题库名称、标识或学科检索…"
              value={keyword}
              onChange={(e) => setKeyword(e.target.value)}
              style={{ maxWidth: 320 }}
            />
          </div>
          <Table
            rowKey="id"
            columns={bankColumns}
            dataSource={filteredBanks}
            scroll={{ x: "max-content" }}
            expandable={{
              expandedRowRender: renderVersionTable,
              defaultExpandAllRows: filteredBanks.length <= 5,
            }}
            pagination={{ pageSize: 10, showSizeChanger: false }}
            locale={{ emptyText: "暂无题库，点击右上角「新建题库」开始" }}
          />
        </Card>
      )}

      {/* 新建题库弹窗 */}
      <Modal
        open={createOpen}
        title="新建题库"
        okText="创建"
        cancelText="取消"
        confirmLoading={busy}
        onOk={() => void handleCreateBank()}
        onCancel={() => setCreateOpen(false)}
      >
        <Form form={createForm} layout="vertical" style={{ marginTop: 12 }}>
          <Form.Item
            name="name"
            label="题库名称"
            rules={[{ required: true, whitespace: true, message: "请输入题库名称" }]}
          >
            <Input maxLength={120} placeholder="例如：计算机网络期末题库" />
          </Form.Item>
          <Form.Item
            name="slug"
            label="唯一标识（Slug）"
            rules={[
              { required: true, message: "请输入唯一标识" },
              { pattern: /^[a-z0-9][a-z0-9-]{1,62}$/, message: "仅支持小写字母、数字与连字符" },
            ]}
          >
            <Input placeholder="例如：computer-networks" />
          </Form.Item>
          <Form.Item name="subject" label="所属学科（选填）">
            <Input maxLength={80} placeholder="例如：计算机科学" />
          </Form.Item>
          <Form.Item name="description" label="题库简介（选填）">
            <Input.TextArea rows={3} maxLength={500} />
          </Form.Item>
        </Form>
      </Modal>

      {/* 导入题库版本抽屉 */}
      <Drawer
        open={Boolean(importBank)}
        title={`导入新版本：${importBank?.name ?? ""}`}
        size={620}
        onClose={() => setImportBank(null)}
        extra={
          <Space>
            <Button onClick={() => setImportBank(null)}>取消</Button>
            <Button
              type="primary"
              loading={busy}
              disabled={!importJson.trim()}
              onClick={() => void handleImportSubmit()}
            >
              校验并导入草稿
            </Button>
          </Space>
        }
      >
        <Space orientation="vertical" size={14} style={{ width: "100%" }}>
          <Alert
            type="info"
            showIcon
            title="导入后会生成一个独立的草稿版本，不会直接覆盖线上题库；经人工审阅校验后再发布上架。"
          />
          <Space wrap align="center">
            <Upload
              accept=".json,application/json"
              maxCount={1}
              showUploadList={false}
              beforeUpload={(file) => {
                void handleImportFile(file);
                return false;
              }}
            >
              <Button icon={<UploadOutlined />}>选择 JSON 题库文件</Button>
            </Upload>
            <Typography.Text type="secondary" style={{ fontSize: 13 }}>
              {importFileName ? `已选择：${importFileName}` : "支持本地选择 .json 文件，或在下方直接粘贴"}
            </Typography.Text>
          </Space>
          <Input
            placeholder="本次版本说明（选填，例如：补充 2026 春季期末真题）"
            value={importNotes}
            onChange={(e) => setImportNotes(e.target.value)}
          />
          <Input.TextArea
            rows={12}
            className="console-mono"
            placeholder='粘贴题目 JSON 数组或含 "questions" 字段的 JSON 对象（单次上限 2 MiB）'
            value={importJson}
            onChange={(e) => setImportJson(e.target.value)}
          />
          {importReport && (
            <Alert
              type="error"
              showIcon
              title="题目校验发现问题，本次未写入"
              description={
                <ul style={{ margin: "6px 0 0", paddingLeft: 18 }}>
                  {(importReport.issues ?? []).map((issue, idx) => (
                    <li key={idx}>
                      {issue.source_question_id ? `[${issue.source_question_id}] ` : ""}
                      {issue.field ? `${issue.field}: ` : ""}
                      {issue.message}
                    </li>
                  ))}
                </ul>
              }
            />
          )}
        </Space>
      </Drawer>

      {/* 版本题目审阅抽屉 */}
      <Drawer
        open={Boolean(previewTarget)}
        title={
          previewTarget
            ? `${previewTarget.bank.name} · v${previewTarget.version.version_number}`
            : "题目审阅"
        }
        size={680}
        onClose={() => setPreviewTarget(null)}
      >
        {previewLoading ? (
          <ModuleStateView state="loading" loadingText="正在加载完整题目列表…" />
        ) : previewDetail ? (
          <Space orientation="vertical" size={14} style={{ width: "100%" }}>
            <Descriptions bordered size="small" column={2}>
              <Descriptions.Item label="版本状态">
                {versionStateTag(
                  previewTarget?.version.active ? "published" : previewDetail.state,
                )}
              </Descriptions.Item>
              <Descriptions.Item label="总题数">
                {previewDetail.questions.length} 题
              </Descriptions.Item>
            </Descriptions>
            {previewDetail.questions.length === 0 ? (
              <Empty description="该版本暂无题目" />
            ) : (
              <Space orientation="vertical" size={10} style={{ width: "100%" }}>
                {previewDetail.questions.slice(0, 100).map((q, index) => (
                  <Card key={q.source_question_id || index} size="small">
                    <Space orientation="vertical" size={6} style={{ width: "100%" }}>
                      <Space size={8}>
                        <Tag>{questionTypeLabel(q.type)}</Tag>
                        <span className="console-code-chip">{q.source_question_id}</span>
                        {q.chapter_name && (
                          <Typography.Text type="secondary">{q.chapter_name}</Typography.Text>
                        )}
                      </Space>
                      <div style={{ marginBottom: 4, lineHeight: 1.65 }}>
                        {renderFormattedStem(q.content)}
                      </div>
                      {q.options && q.options.length > 0 && (
                        <div style={{ paddingLeft: 12, fontSize: 13, color: "#475569" }}>
                          {q.options.map((opt, oi) => (
                            <div key={oi}>
                              {String.fromCharCode(65 + oi)}. {opt}
                            </div>
                          ))}
                        </div>
                      )}
                      <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                        参考答案：{formatQuestionAnswer(q)}
                      </Typography.Text>
                      {q.explanation && (
                        <div
                          style={{
                            marginTop: 4,
                            padding: "6px 10px",
                            background: "#f8fafc",
                            borderRadius: 6,
                            fontSize: 12,
                            color: "#475569",
                          }}
                        >
                          <strong>解析：</strong>
                          {renderFormattedStem(q.explanation)}
                        </div>
                      )}
                    </Space>
                  </Card>
                ))}
                {previewDetail.questions.length > 100 && (
                  <Typography.Text type="secondary">
                    当前仅展示前 100 题（共 {previewDetail.questions.length} 题）。
                  </Typography.Text>
                )}
              </Space>
            )}
          </Space>
        ) : null}
      </Drawer>

      {/* 生命周期确认弹窗 */}
      <Modal
        open={Boolean(lifecycleModal)}
        title={
          lifecycleModal
            ? {
                validate: "确认标记为已通过人工校验",
                publish: "确认发布该题库版本上架",
                unpublish: "确认下架该题库版本",
                rollback: "确认回滚至该历史版本",
              }[lifecycleModal.action]
            : ""
        }
        okText="确认执行"
        cancelText="取消"
        okButtonProps={{ danger: lifecycleModal?.action === "unpublish" }}
        confirmLoading={busy}
        onOk={() => void handleLifecycleSubmit()}
        onCancel={() => setLifecycleModal(null)}
      >
        {lifecycleModal && (
          <Space orientation="vertical" size={12} style={{ width: "100%", marginTop: 8 }}>
            <Typography.Text>
              目标题库：<strong>{lifecycleModal.bank.name}</strong> · 版本{" "}
              <strong>v{lifecycleModal.version.version_number}</strong>
            </Typography.Text>
            <Input.TextArea
              rows={3}
              placeholder="备注本次操作说明（选填）"
              value={lifecycleNote}
              onChange={(e) => setLifecycleNote(e.target.value)}
            />
          </Space>
        )}
      </Modal>
    </section>
  );
}
