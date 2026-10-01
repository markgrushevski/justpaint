import { test as base, expect, type Page, type Route } from '@playwright/test'

/**
 * Flow tests for the /draw file actions (save, open, new, leave, account change) in a
 * real Chromium. The API is faked in the browser by a small in-memory store that keeps
 * rows per owner and records every request, so a test can assert on what the editor sent
 * and in which order. A response can be held back to drive the races between a slow
 * request and what the visitor does meanwhile.
 *
 * Run: `npm run test:flows -w @justpaint/web` (needs the Vite dev server).
 */

interface FakeUser {
    id: string
    login: string
    displayName: string | null
    rating: number
    createdAt: string
}

const ALICE: FakeUser = {
    id: 'u-alice',
    login: 'alice',
    displayName: null,
    rating: 1000,
    createdAt: '2026-09-01T10:00:00Z'
}
const BOB: FakeUser = { id: 'u-bob', login: 'bob', displayName: null, rating: 1000, createdAt: '2026-09-02T10:00:00Z' }
const USERS = [ALICE, BOB]

const STAMP = '2026-09-29T09:00:00Z'

const PEN = {
    id: 'stk1',
    type: 'freehand',
    composite: 'source-over',
    color: '#1b1b1b',
    points: [
        [80, 150, 0.5],
        [160, 90, 0.5],
        [240, 210, 0.5],
        [320, 130, 0.5]
    ],
    brush: {
        size: 16,
        thinning: 0.5,
        smoothing: 0.5,
        streamline: 0.5,
        simulatePressure: true,
        taperStart: 0,
        taperEnd: 0
    }
}

const SAVED_DOCUMENT = {
    version: 1,
    width: 400,
    height: 300,
    background: null,
    layers: [{ id: 'l1', name: 'Layer 1', visible: true, opacity: 1, strokes: [PEN] }]
}

interface Row {
    id: string
    owner: string
    name: string
    document: unknown
}

interface Call {
    method: string
    path: string
    body: Record<string, unknown> | null
}

interface Reply {
    status: number
    json?: unknown
}

interface Hold {
    method: string
    path: string
    taken: boolean
    released: Promise<void>
    release: () => void
}

const API_URL = /^https?:\/\/[^/]+\/api\//

function metaOf(row: Row) {
    return {
        id: row.id,
        ownerId: row.owner,
        matchId: null,
        name: row.name,
        docVersion: 1,
        width: 400,
        height: 300,
        thumbnailUrl: null,
        createdAt: STAMP,
        updatedAt: STAMP
    }
}

function failure(status: number, code: string): Reply {
    return { status, json: { error: { code, message: code.replace('_', ' ') } } }
}

function parseBody(raw: string | null): Record<string, unknown> | null {
    if (!raw) return null
    try {
        return JSON.parse(raw) as Record<string, unknown>
    } catch {
        return null
    }
}

/** The auth and drawings routes of the Go server, held in memory. */
class FakeApi {
    /** Who the session cookie belongs to; null answers 401. */
    user: FakeUser | null
    readonly calls: Call[] = []
    readonly rows = new Map<string, Row>()
    private readonly holds: Hold[] = []
    private created = 0

    constructor(user: FakeUser | null) {
        this.user = user
    }

    seed(row: Row): void {
        this.rows.set(row.id, row)
    }

    /** Delays the response to the next matching request until the returned function runs. */
    hold(method: string, path: string): () => void {
        let release: () => void = () => undefined
        const released = new Promise<void>((resolve) => (release = resolve))
        this.holds.push({ method, path, taken: false, released, release })
        return release
    }

    releaseAll(): void {
        for (const hold of this.holds) hold.release()
    }

    count(method: string, path: string): number {
        return this.calls.filter((c) => c.method === method && c.path === path).length
    }

    /** `METHOD /path` for every drawings request so far, in order. */
    drawingRequests(): string[] {
        return this.calls.filter((c) => c.path.startsWith('/api/drawings')).map((c) => `${c.method} ${c.path}`)
    }

    async handle(route: Route): Promise<void> {
        const request = route.request()
        const path = new URL(request.url()).pathname
        const method = request.method()
        const body = parseBody(request.postData())
        this.calls.push({ method, path, body })

        const reply = this.respond(method, path, body)
        const hold = this.holds.find((h) => !h.taken && h.method === method && h.path === path)
        if (hold) {
            hold.taken = true
            await hold.released
        }
        // The page may be gone by the time a held response is released.
        await route.fulfill({ status: reply.status, json: reply.json }).catch(() => undefined)
    }

    private respond(method: string, path: string, body: Record<string, unknown> | null): Reply {
        if (path === '/api/auth/me' && method === 'GET') {
            return this.user ? { status: 200, json: { user: this.user } } : failure(401, 'unauthorized')
        }
        if (path === '/api/auth/login' && method === 'POST') {
            const found = USERS.find((u) => u.login === body?.login)
            if (!found) return failure(401, 'invalid_credentials')
            this.user = found
            return { status: 200, json: { user: found } }
        }
        if (path === '/api/auth/logout' && method === 'POST') {
            this.user = null
            return { status: 204 }
        }

        const match = /^\/api\/drawings(?:\/([^/]+))?$/.exec(path)
        if (!match) return failure(404, 'not_found')
        const user = this.user
        if (!user) return failure(401, 'unauthorized')

        const id = match[1]
        if (!id) {
            if (method === 'GET') {
                const mine = [...this.rows.values()].filter((r) => r.owner === user.id)
                return { status: 200, json: { drawings: mine.map(metaOf), nextCursor: null, limit: 24 } }
            }
            if (method === 'POST') {
                const name = typeof body?.name === 'string' && body.name ? body.name : 'new art'
                const row: Row = { id: `row-${++this.created}`, owner: user.id, name, document: body?.document }
                this.rows.set(row.id, row)
                return { status: 201, json: { drawing: metaOf(row) } }
            }
            return failure(404, 'not_found')
        }

        // Ownership-scoped: a foreign row answers 404, as the server does.
        const row = this.rows.get(id)
        if (!row || row.owner !== user.id) return failure(404, 'not_found')
        if (method === 'GET') return { status: 200, json: { drawing: { ...metaOf(row), document: row.document } } }
        if (method === 'PUT') {
            row.document = body?.document
            if (typeof body?.name === 'string' && body.name) row.name = body.name
            return { status: 200, json: { drawing: metaOf(row) } }
        }
        if (method === 'DELETE') {
            this.rows.delete(id)
            return { status: 204 }
        }
        return failure(404, 'not_found')
    }
}

const test = base.extend<{ api: FakeApi }>({
    api: async ({ page }, use) => {
        const api = new FakeApi(ALICE)
        await page.route(API_URL, (route) => api.handle(route))
        await use(api)
        api.releaseAll()
    }
})

const SUNSET: Row = { id: 'd1', owner: ALICE.id, name: 'Sunset study', document: SAVED_DOCUMENT }

/** The toolbar's Undo: enabled exactly when the canvas holds a stroke that can be undone. */
function undoButton(page: Page) {
    return page.locator('.bar').getByRole('button', { name: 'Undo' })
}

async function openDraw(page: Page, url = '/draw'): Promise<void> {
    await page.goto(url)
    await page.locator('.bar').waitFor({ state: 'visible' })
    await page.locator('.konvajs-content canvas').first().waitFor({ state: 'visible' })
}

/** Drags the pen across the canvas, clear of the chrome; `row` shifts the stroke down to tell strokes apart. */
async function drawStroke(page: Page, row = 0): Promise<void> {
    const box = await page.locator('.shell__canvas').boundingBox()
    if (!box) throw new Error('The canvas has no layout box.')
    const x = box.x + box.width * 0.5
    const y = box.y + box.height * 0.4 + row * 40
    await page.mouse.move(x, y)
    await page.mouse.down()
    await page.mouse.move(x + 90, y + 40, { steps: 8 })
    await page.mouse.move(x + 180, y - 20, { steps: 8 })
    await page.mouse.up()
    await expect(undoButton(page)).not.toHaveAttribute('aria-disabled', 'true')
}

const saveDialog = (page: Page) => page.getByRole('dialog', { name: 'Save drawing' })

/** Ctrl+S, name the drawing in the dialog it opens and confirm. */
async function saveAs(page: Page, name: string): Promise<void> {
    await page.keyboard.press('Control+s')
    const dialog = saveDialog(page)
    await dialog.getByLabel('Name').fill(name)
    await dialog.getByRole('button', { name: 'Save' }).click()
    await expect(dialog).toBeHidden()
}

/** The save finished: its toast is up and the Save button is no longer busy. */
async function expectSaved(page: Page, name: string): Promise<void> {
    await expect(page.getByText(`Saved “${name}”.`)).toBeVisible()
    await expect(page.locator('.draw__save')).not.toHaveAttribute('aria-busy', 'true')
}

async function openMenu(page: Page): Promise<void> {
    await page.getByRole('button', { name: 'Open menu' }).click()
    await page.locator('aside.menu.menu--open').waitFor({ state: 'visible' })
    await expect(page.locator('aside.menu')).toHaveJSProperty('inert', false)
}

test.describe('/draw file actions', () => {
    test('the first save asks for a name; the next one updates in place', async ({ page, api }) => {
        await openDraw(page)
        await drawStroke(page)

        await saveAs(page, 'First sketch')
        await expectSaved(page, 'First sketch')

        const created = [...api.rows.values()]
        expect(created).toHaveLength(1)
        expect(created[0].name).toBe('First sketch')
        const id = created[0].id

        await page.keyboard.press('Control+s')
        await expect.poll(() => api.count('PUT', `/api/drawings/${id}`)).toBe(1)
        await expect(saveDialog(page)).toBeHidden()

        expect(api.drawingRequests()).toEqual(['POST /api/drawings', `PUT /api/drawings/${id}`])
        const [create, update] = api.calls.filter((c) => c.path.startsWith('/api/drawings'))
        expect(create.body).toHaveProperty('name', 'First sketch')
        expect(update.body).not.toBeNull()
        expect(update.body).not.toHaveProperty('name')
    })

    test('a save that lands after "New drawing" does not bind the new canvas', async ({ page, api }) => {
        await openDraw(page)
        await drawStroke(page)

        const release = api.hold('POST', '/api/drawings')
        await saveAs(page, 'Late save')
        await expect.poll(() => api.count('POST', '/api/drawings')).toBe(1)

        // The save is still in flight: start over.
        await openMenu(page)
        await page.getByRole('button', { name: 'New drawing' }).click()
        await page.getByRole('dialog', { name: 'Clear the canvas?' }).getByRole('button', { name: 'Clear' }).click()
        await expect(undoButton(page)).toHaveAttribute('aria-disabled', 'true')
        await drawStroke(page, 1)

        release()
        await expectSaved(page, 'Late save')

        // The new canvas is unsaved, so this asks for a name and creates; it must not update the old row.
        await page.keyboard.press('Control+s')
        await expect(saveDialog(page)).toBeVisible()
        expect(api.drawingRequests()).toEqual(['POST /api/drawings'])
    })

    test('/draw?id= opens the saved drawing and drops the id from the URL', async ({ page, api }) => {
        api.seed(SUNSET)
        await openDraw(page, '/draw?id=d1')

        await expect(page).toHaveURL(/\/draw$/)
        await expect.poll(() => api.count('GET', '/api/drawings/d1')).toBe(1)

        await openMenu(page)
        await expect(page.locator('.menu__name')).toHaveText('Sunset study')
    })

    test('strokes drawn while the drawing loads are kept', async ({ page, api }) => {
        api.seed(SUNSET)
        const release = api.hold('GET', '/api/drawings/d1')
        await openDraw(page, '/draw?id=d1')
        await expect.poll(() => api.count('GET', '/api/drawings/d1')).toBe(1)

        await drawStroke(page)
        release()

        await expect(page.getByText(/Kept your new strokes/)).toBeVisible()
        // Opening the drawing would have replaced the canvas and dropped the history.
        await expect(undoButton(page)).not.toHaveAttribute('aria-disabled', 'true')
        await openMenu(page)
        await expect(page.locator('.menu__name')).toHaveText('Unsaved drawing')
    })

    test('leaving with an unsaved stroke asks first; a saved drawing leaves freely', async ({ page, api }) => {
        await openDraw(page)
        await drawStroke(page)

        const practice = page.getByRole('navigation', { name: 'Modes' }).getByRole('link', { name: 'Practice' })
        const question = page.getByRole('dialog', { name: 'Leave without saving?' })

        await practice.click()
        await expect(question).toBeVisible()
        await question.getByRole('button', { name: 'Stay' }).click()
        await expect(question).toBeHidden()
        await expect(page).toHaveURL(/\/draw$/)

        await saveAs(page, 'Kept sketch')
        await expectSaved(page, 'Kept sketch')
        expect(api.rows.size).toBe(1)

        await practice.click()
        await expect(page).toHaveURL(/\/practice$/)
        await expect(question).toHaveCount(0)
    })

    test('after an account switch the drawing is unsaved again and saving creates a row', async ({ page, api }) => {
        await openDraw(page)
        await drawStroke(page)
        await saveAs(page, 'Alice art')
        await expectSaved(page, 'Alice art')
        const [aliceRow] = [...api.rows.values()]

        // A -> signed out -> B: a watch on the user going from A to B never sees this.
        await openMenu(page)
        await page.getByRole('button', { name: 'Log out' }).click()
        await page.getByRole('button', { name: 'Sign in' }).click()
        const signIn = page.getByRole('dialog', { name: 'Sign in' })
        await signIn.getByLabel('Login').fill(BOB.login)
        await signIn.getByLabel('Password').fill('not-a-real-password')
        await signIn.getByRole('button', { name: 'Log in' }).click()
        await expect(signIn).toBeHidden()

        await page.keyboard.press('Control+s')
        await expect(saveDialog(page)).toBeVisible()
        expect(api.calls.filter((c) => c.method === 'PUT')).toEqual([])

        await saveDialog(page).getByLabel('Name').fill('Bob art')
        await saveDialog(page).getByRole('button', { name: 'Save' }).click()
        await expectSaved(page, 'Bob art')

        expect(api.drawingRequests()).toEqual(['POST /api/drawings', 'POST /api/drawings'])
        const bobRow = [...api.rows.values()].find((r) => r.name === 'Bob art')
        expect(bobRow?.owner).toBe(BOB.id)
        expect(bobRow?.id).not.toBe(aliceRow.id)
    })
})
