/**
 * Post-deploy smoke: drive a whole duel through a running deployment.
 *
 *     npm run smoke -- https://justpaint.onrender.com
 *
 * CI proves the image boots and serves; this proves the PRODUCT works — auth and
 * the session cookie's flags, matchmaking, prompt reveal, the document validator
 * at the submit edge, the authoritative server-side render (RENDER_MODE=node,
 * two node-canvas child processes — the most fragile thing in a small
 * deployment), the judge, Elo, the post-result reveal, and the 404-not-403
 * boundary a non-player must hit.
 *
 * IT CREATES REAL ACCOUNTS. Against a public deployment they appear on the
 * leaderboard until removed. There is no delete-user endpoint by design, so
 * clean up in SQL (order matters — nothing cascades):
 *
 *     delete from match_players where user_id in (select id from users where login like 'smoke-%');
 *     delete from drawings      where owner_id in (select id from users where login like 'smoke-%');
 *     -- every match now missing both seats is one of the above; a real match
 *     -- always keeps at least its creator, so this cannot reach a live one.
 *     delete from matches       where id not in (select match_id from match_players);
 *     delete from users         where login like 'smoke-%';
 */
const BASE = (process.argv[2] ?? 'http://localhost:8080').replace(/\/$/, '')
const stamp = Date.now().toString(36)
const password = `smoke-${stamp}-${Math.random().toString(36).slice(2)}`

let failures = 0
const ok = (label, cond, detail = '') => {
    if (!cond) failures++
    console.log(`${cond ? 'PASS' : 'FAIL'}  ${label}${detail ? ' — ' + detail : ''}`)
}
const sleep = (ms) => new Promise((r) => setTimeout(r, ms))

/** One browser-like session: its own cookie jar of exactly one cookie. */
const session = (label) => ({ label, cookie: null, user: null })

async function call(s, method, path, body) {
    const res = await fetch(`${BASE}${path}`, {
        method,
        headers: { 'content-type': 'application/json', ...(s.cookie ? { cookie: s.cookie } : {}) },
        body: body === undefined ? undefined : JSON.stringify(body)
    })
    const setCookie = res.headers.get('set-cookie')
    if (setCookie) s.cookie = setCookie.split(';')[0]
    const text = await res.text()
    let json = null
    try {
        json = JSON.parse(text)
    } catch {
        json = { raw: text.slice(0, 200) }
    }
    return { status: res.status, json, setCookie }
}

/** A real 1080² document: the canvas size the match contract demands. */
const document = (points, n) => ({
    version: 1,
    width: 1080,
    height: 1080,
    background: '#ffffff',
    layers: [
        {
            id: `lyr_${stamp}_${n}`,
            name: 'Layer 1',
            visible: true,
            opacity: 1,
            strokes: [
                {
                    id: `stk_${stamp}_${n}`,
                    type: 'freehand',
                    composite: 'source-over',
                    color: '#1b1b1b',
                    points,
                    brush: {
                        size: 24,
                        thinning: 0.5,
                        smoothing: 0.5,
                        streamline: 0.5,
                        simulatePressure: true,
                        taperStart: 0,
                        taperEnd: 0
                    }
                }
            ]
        }
    ]
})

console.log(`smoke: ${BASE}\n`)

// --- readiness ---------------------------------------------------------------
const anon = session('anon')
const ready = await call(anon, 'GET', '/readyz')
ok('readiness reports a migrated schema', ready.status === 200, JSON.stringify(ready.json))
const shell = await fetch(`${BASE}/leaderboard`)
ok('a client route serves the SPA shell', (await shell.text()).toLowerCase().includes('<!doctype html'))
const guarded = await call(anon, 'GET', '/api/leaderboard')
ok('the API refuses an anonymous caller', guarded.status === 401, `status ${guarded.status}`)

// --- auth ---------------------------------------------------------------------
const a = session('A')
const b = session('B')
for (const s of [a, b]) {
    const res = await call(s, 'POST', '/api/auth/register', {
        login: `smoke-${s.label.toLowerCase()}-${stamp}`,
        password,
        displayName: `Smoke ${s.label}`
    })
    ok(`register ${s.label}`, res.status === 201, `status ${res.status}`)
    s.user = res.json?.user
    if (s.label === 'A') {
        const c = res.setCookie ?? ''
        ok(
            'the session cookie is HttpOnly + Secure + SameSite',
            /HttpOnly/i.test(c) && /Secure/i.test(c) && /SameSite/i.test(c),
            c.replace(/=[^;]+/, '=…')
        )
        ok('the password is never echoed', !JSON.stringify(res.json).includes(password))
    }
}

// --- the duel -----------------------------------------------------------------
const created = await call(a, 'POST', '/api/matches')
const matchId = created.json?.match?.id
ok('A opens a match', Boolean(matchId), `status ${created.status}`)

const joined = await call(b, 'POST', '/api/matches')
ok('B auto-joins that same match', joined.json?.match?.id === matchId)
ok('the round starts drawing', joined.json?.match?.status === 'drawing', joined.json?.match?.status)
ok('the prompt is revealed to a player', Boolean(joined.json?.match?.prompt?.text), joined.json?.match?.prompt?.text)

const midRound = await call(a, 'GET', `/api/matches/${matchId}`)
ok(
    'mid-round state hides the opponent canvas',
    (JSON.stringify(midRound.json).match(/"drawingId":"[^"]+"/g) ?? []).length === 0
)

const wave = (phase) => Array.from({ length: 40 }, (_, i) => [180 + i * 16, 420 + Math.sin(i / 4 + phase) * 140, 0.5])
ok(
    'A submits',
    (await call(a, 'POST', `/api/matches/${matchId}/submit`, { document: document(wave(0), 1) })).status === 202
)
const lastSubmit = await call(b, 'POST', `/api/matches/${matchId}/submit`, { document: document(wave(2), 2) })
ok('B submits', lastSubmit.status === 202, `status ${lastSubmit.status}`)
ok(
    'the final submit moves the match to judging',
    lastSubmit.json?.match?.status === 'judging',
    lastSubmit.json?.match?.status
)

// --- the verdict ---------------------------------------------------------------
let result = null
const startedAt = Date.now()
for (let i = 0; i < 45 && !result; i++) {
    const res = await call(a, 'GET', `/api/matches/${matchId}/result`)
    // The payload is nested under `result`; reading `ready` off the envelope is
    // how an early version of this script accused production of hanging.
    const payload = res.json?.result ?? res.json
    if (payload?.ready) result = payload
    else await sleep(2000)
}
ok(
    'a verdict arrives',
    Boolean(result),
    result ? `${((Date.now() - startedAt) / 1000).toFixed(1)}s` : 'timed out after 90s'
)

if (result) {
    ok(
        'the match is done and was judged',
        result.status === 'done' && result.resolution === 'judged',
        `${result.status}/${result.resolution}`
    )
    ok('the judge left a player-facing reason', Boolean(result.reason), result.reason)

    const players = result.players ?? []
    ok(
        'both canvases were rendered and scored server-side',
        players.length === 2 && players.every((p) => typeof p.score === 'number'),
        players.map((p) => p.score?.toFixed(4)).join(' vs ')
    )

    // Elo is only *required* to move when the duel had a winner. A tie between
    // equal ratings must leave both untouched — asserting "it moved" there is
    // asserting the formula is wrong.
    const [p1, p2] = players
    const delta = (p) => p.ratingAfter - p.ratingBefore
    if (result.isTie && p1?.ratingBefore === p2?.ratingBefore) {
        ok(
            'a tie between equal ratings moves nobody',
            delta(p1) === 0 && delta(p2) === 0,
            `${delta(p1)} / ${delta(p2)}`
        )
    } else {
        ok(
            'Elo moved in opposite directions',
            delta(p1) === -delta(p2) && delta(p1) !== 0,
            `${delta(p1)} / ${delta(p2)}`
        )
    }

    const reveal = await call(a, 'GET', `/api/matches/${matchId}/players/${b.user.id}/drawing`)
    const revealed = reveal.json?.drawing?.document ?? reveal.json?.document
    ok(
        'the opponent canvas is revealed only now',
        reveal.status === 200 && revealed?.width === 1080,
        `status ${reveal.status}`
    )
}

// --- the boundary ---------------------------------------------------------------
const stranger = session('C')
await call(stranger, 'POST', '/api/auth/register', { login: `smoke-c-${stamp}`, password })
const trespass = await call(stranger, 'GET', `/api/matches/${matchId}`)
ok('a non-player gets 404, never 403', trespass.status === 404, `status ${trespass.status}`)

console.log(
    `\n${failures === 0 ? 'ALL PASS' : `${failures} FAILED`} — accounts created: smoke-{a,b,c}-${stamp} (see the header for cleanup SQL)`
)
process.exit(failures === 0 ? 0 : 1)
