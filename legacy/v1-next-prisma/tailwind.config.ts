import type { Config } from "tailwindcss";

const config: Config = {
  content: [
    "./src/app/**/*.{js,ts,jsx,tsx,mdx}",
    "./src/components/**/*.{js,ts,jsx,tsx,mdx}",
    "./src/constants/**/*.{js,ts,jsx,tsx,mdx}",
  ],
  theme: {
    extend: {
      colors: {
        ink: "#172026",
        muted: "#5f6f7a",
        line: "#d8e0e5",
        panel: "#f7faf9",
        brand: "#176c5f",
        accent: "#b55a30",
      },
      boxShadow: {
        soft: "0 12px 28px rgba(18, 38, 63, 0.08)",
      },
    },
  },
  plugins: [],
};

export default config;

