import { accessLevelLabels } from "@/constants/enums";
import type { AccessLevel } from "@/types";

const badgeClassName: Record<AccessLevel, string> = {
  free: "border-emerald-200 bg-emerald-50 text-emerald-800",
  login_required: "border-sky-200 bg-sky-50 text-sky-800",
  paid: "border-amber-200 bg-amber-50 text-amber-800",
};

export function AccessBadge({ accessLevel }: { accessLevel: AccessLevel }) {
  return (
    <span
      className={`inline-flex h-7 items-center rounded-md border px-2.5 text-xs font-semibold ${badgeClassName[accessLevel]}`}
    >
      {accessLevelLabels[accessLevel]}
    </span>
  );
}

