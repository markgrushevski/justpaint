import { defineConfig, devices } from '@playwright/test'

/**
 * Real-browser accessibility layer (@playwright/test + @axe-core/playwright).
 *
 * This catches RENDERED a11y issues — contrast in context, accessible names,
 * roles, focus — that the token-contrast lint (`lint:contrast`) and the
 * happy-dom unit tests structurally can't see. It is deliberately NOT wired
 * into `lint:all`: a headless browser run is heavy, so it stays its own
 * command (`npm run test:a11y`). See `tests/a11y/draw.spec.ts`.
 */
export default defineConfig({
    testDir: 'tests/a11y',
    // A single audited route today (/draw); serial keeps the shared dev server
    // and the output readable.
    fullyParallel: false,
    forbidOnly: !!process.env.CI,
    retries: 0,
    workers: 1,
    reporter: [['list']],
    timeout: 60_000,
    expect: { timeout: 10_000 },
    use: {
        baseURL: 'http://localhost:7777',
        headless: true,
        // 1280x800 desktop default; the mobile viewport is set per-test.
        viewport: { width: 1280, height: 800 },
        trace: 'on-first-retry'
    },
    projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'], viewport: { width: 1280, height: 800 } } }],
    // The command runs with this config's directory (apps/web) as cwd, so the
    // bare workspace `dev` script is correct. Reuses the Vite server already on
    // :7777 (e.g. the preview tool) instead of spawning a second one.
    webServer: {
        command: 'npm run dev',
        url: 'http://localhost:7777/draw',
        reuseExistingServer: true,
        timeout: 120_000
    }
})
