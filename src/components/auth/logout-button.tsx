"use client";

import { useState } from "react";

export function LogoutButton() {
  const [isLoading, setIsLoading] = useState(false);

  async function handleLogout() {
    setIsLoading(true);
    await fetch("/api/auth/logout", { method: "POST" });
    window.location.href = "/";
  }

  return (
    <button
      type="button"
      onClick={handleLogout}
      disabled={isLoading}
      className="rounded-md border border-line px-3 py-2 text-sm font-semibold text-ink hover:bg-panel disabled:cursor-not-allowed disabled:text-muted focus-ring"
    >
      {isLoading ? "退出中" : "退出"}
    </button>
  );
}

