import type { Config } from "tailwindcss";

const config: Config = {
  content: ["./app/**/*.{ts,tsx}", "./components/**/*.{ts,tsx}", "./lib/**/*.{ts,tsx}"],
  theme: {
    extend: {
      borderRadius: {
        xl: "0.875rem",
        "2xl": "1rem",
      },
      boxShadow: {
        soft: "0 18px 60px rgba(15, 23, 42, 0.08)",
      },
      colors: {
        background: "#f7f8f5",
        foreground: "#18181b",
        muted: "#f1f3ee",
        "muted-foreground": "#6b7280",
        card: "#ffffff",
        border: "#dfe4dc",
        primary: "#2f5f51",
        "primary-foreground": "#ffffff",
        accent: "#edf6f1",
        "accent-foreground": "#23483d",
        ink: "#18181b",
        paper: "#f7f8f5",
        sage: "#2f5f51",
        line: "#dfe4dc",
      },
    },
  },
  plugins: [],
};

export default config;
