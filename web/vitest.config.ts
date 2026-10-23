import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
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
