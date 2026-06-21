import { getCurrentUser } from "@/lib/auth";
import { LogoutButton } from "@/components/auth/logout-button";

export async function AuthStatus() {
  const user = await getCurrentUser();

  if (!user) {
    return (
      <a
        href="/login"
        className="rounded-md bg-brand px-3 py-2 font-semibold text-white hover:bg-[#12574d] focus-ring"
      >
        学生邮箱登录
      </a>
    );
  }

  return (
    <div className="flex flex-wrap items-center gap-2">
      <span className="rounded-md bg-panel px-3 py-2 text-sm font-semibold text-ink">
        {user.email}
      </span>
      <LogoutButton />
    </div>
  );
}

