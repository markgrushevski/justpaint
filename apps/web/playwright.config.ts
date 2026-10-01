import { defineConfig, devices } from '@playwright/test'

/**
 * Real-browser layer for what static gates can't see: rendered a11y
 * (`tests/a11y`, docs/DECISIONS.md), rendered geometry (`tests/layout`,
 * docs/NOTES.md) and the /draw file flows against a mocked API (`tests/flows`).
 * All need a live dev server, so they stay their own local commands
 * (`test:a11y`, `test:layout`, `test:flows`) rather than joining `lint:all`.
 */
export default defineConfig({
    testDir: 'tests',
    // Serial keeps the shared dev server and the output readable.
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
