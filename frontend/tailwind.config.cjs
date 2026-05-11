/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  darkMode: "class",
  theme: {
    extend: {
      colors: {
        // Cam2You brand ranglari (Hikvision'ni hurmat qilgan holda)
        bg: {
          DEFAULT: "#0f1419",
          card: "#1a2030",
          subtle: "#252b3b",
        },
        accent: {
          DEFAULT: "#3b82f6", // ko'k
          hover: "#2563eb",
        },
        success: "#10b981",
        warning: "#f59e0b",
        danger: "#ef4444",
      },
      fontFamily: {
        sans: ["Inter", "system-ui", "sans-serif"],
        mono: ["JetBrains Mono", "Consolas", "monospace"],
      },
    },
  },
  plugins: [],
};
