import Link from "next/link";
import type { ComponentPropsWithoutRef, ReactNode } from "react";

type ButtonVariant = "primary" | "secondary" | "ghost";

const variantClass: Record<ButtonVariant, string> = {
  primary: "bg-primary text-primary-foreground shadow-sm hover:bg-[#254d42]",
  secondary: "border border-border bg-card text-foreground hover:bg-muted",
  ghost: "text-muted-foreground hover:bg-muted hover:text-foreground",
};

type ButtonLinkProps = Omit<ComponentPropsWithoutRef<typeof Link>, "key" | "ref"> & {
  children: ReactNode;
  variant?: ButtonVariant;
};

export function ButtonLink({ children, className = "", variant = "primary", ...props }: ButtonLinkProps) {
  return (
    <Link
      className={`inline-flex items-center justify-center rounded-xl px-4 py-2 text-sm font-medium transition ${variantClass[variant]} ${className}`}
      {...props}
    >
      {children}
    </Link>
  );
}
