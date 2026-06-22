"use client";

import { useEffect, useState } from "react";

export function usePrefersReducedMotion() {
  const [prefersReducedMotion, setPrefersReducedMotion] = useState(false);

  useEffect(() => {
    const query = window.matchMedia("(prefers-reduced-motion: reduce)");
    const updatePreference = () => setPrefersReducedMotion(query.matches);

    updatePreference();
    query.addEventListener("change", updatePreference);

    return () => {
      query.removeEventListener("change", updatePreference);
    };
  }, []);

  return prefersReducedMotion;
}
