import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    environment: 'jsdom',
    include: ['src/**/*.test.ts'],
    globals: true,
    setupFiles: [],
    // Note: singleFork avoids Tinypool minThreads/maxThreads conflict
    // that can occur with default pool + maxWorkers CLI override.
    // This trades module isolation for stability; wrapper's --maxWorkers
    // still limits overall concurrency.
    pool: 'forks',
    poolOptions: {
      forks: {
        singleFork: true,
      },
    },
  },
});
