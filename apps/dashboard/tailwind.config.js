/** @type {import('tailwindcss').Config} */
export default {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        ink: { 950: "#0a0e13", 900: "#0e131a", 800: "#141b24", 700: "#1c2531", 600: "#273343" },
        accent: { DEFAULT: "#22d3ee", blue: "#3b82f6", violet: "#8b5cf6" },
        ok: "#34d399", warn: "#fbbf24", bad: "#f87171",
      },
      fontFamily: { sans: ["Inter", "system-ui", "Segoe UI", "sans-serif"], mono: ["JetBrains Mono", "Consolas", "monospace"] },
    },
  },
  plugins: [],
};
