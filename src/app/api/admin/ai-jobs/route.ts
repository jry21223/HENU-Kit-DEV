import { NextResponse } from "next/server";
import { requireAdminResponse } from "@/lib/admin";
import { getCurrentUser } from "@/lib/auth";
import { parseAiJobStatus } from "@/lib/ai-jobs";
import { createAiJob, listAiJobs } from "@/services/ai-job-service";

type CreateAiJobBody = {
  course_id?: string;
  input_material_ids?: string[] | string;
  output_type?: string;
  simulate_failure?: boolean;
};

function normalizeMaterialIds(value: CreateAiJobBody["input_material_ids"]) {
  if (Array.isArray(value)) {
    return value.map((item) => item.trim()).filter(Boolean);
  }
  if (typeof value === "string") {
    return value
      .split(",")
      .map((item) => item.trim())
      .filter(Boolean);
  }
  return [];
}

export async function GET(request: Request) {
  const forbidden = await requireAdminResponse();
  if (forbidden) return forbidden;

  const { searchParams } = new URL(request.url);
  const status = parseAiJobStatus(searchParams.get("status"));
  if (searchParams.get("status") && !status) {
    return NextResponse.json({ error: "AI 任务状态无效。" }, { status: 400 });
  }

  const jobs = await listAiJobs(status);
  return NextResponse.json({ jobs });
}

export async function POST(request: Request) {
  const forbidden = await requireAdminResponse();
  if (forbidden) return forbidden;
  const admin = await getCurrentUser();

  const body = (await request.json().catch(() => null)) as CreateAiJobBody | null;
  if (!body?.course_id || !body.output_type) {
    return NextResponse.json({ error: "缺少 AI 任务必填字段。" }, { status: 400 });
  }

  const result = await createAiJob({
    adminId: admin?.id ?? "",
    courseId: body.course_id,
    outputType: body.output_type,
    inputMaterialIds: normalizeMaterialIds(body.input_material_ids),
    simulateFailure: body.simulate_failure,
  });

  if (!result.ok) {
    return NextResponse.json({ error: result.message }, { status: result.status });
  }

  return NextResponse.json({ job: result.job }, { status: 201 });
}
