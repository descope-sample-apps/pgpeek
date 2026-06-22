import { defineConfig } from "vitest/config";

export default defineConfig({
  test: {
    environment: "jsdom",
    include: ["web/**/*.test.js"],
    coverage: {
      provider: "v8",
      include: ["web/app.js"],
      reporter: ["text", "html"],
      thresholds: {
        lines: 100,
        functions: 100,
        branches: 100,
        statements: 100,
      },
    },
  },
});
