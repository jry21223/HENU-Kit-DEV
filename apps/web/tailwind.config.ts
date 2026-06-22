import type { Config } from "tailwindcss";

const config: Config = {
  content: ["./app/**/*.{ts,tsx}", "./components/**/*.{ts,tsx}", "./lib/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        ink: "#17202a",
        paper: "#f8faf7",
        sage: "#4f6f64",
        line: "#d8ded6",
      },
    },
  },
  plugins: [],
};

export default config;
