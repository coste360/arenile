import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    // The admin pages render whole feature trees with many queries: beyond
    // ~8 workers the runs contend for CPU and the slowest tests time out.
    maxWorkers: '50%',
    projects: [
      'shared',
      'admin',
      'booking',
      {
        // Node build scripts (web/scripts), e.g. the beach map kit generator.
        test: { name: 'scripts', environment: 'node', include: ['scripts/**/*.test.mjs'] },
      },
    ],
  },
});
