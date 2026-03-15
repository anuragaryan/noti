import { defineConfig } from 'vitest/config'

export default defineConfig({
  test: {
    environment: 'jsdom',
    include: ['src/**/*.integration.test.ts'],
    clearMocks: true,
    restoreMocks: true,
  },
})
