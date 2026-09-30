/**
 * The match socket (docs/API.md §9): opens, pings, and reconnects with backoff. It only
 * speeds the round up; the view's poll loop keeps the round moving while it is down.
 */
import { ref } from 'vue'
import { openMatchSocket } from '@core'
import type { MatchSocketHandle, WsFrame } from '@core'

// Notices a half-open socket without a TCP timeout; the server's WS_READ_IDLE_TIMEOUT
// must clear it.
const PING_MS = 25000

// Capped at the last entry; resets on a clean reconnect.
const RECONNECT_BACKOFF_MS = [1000, 2000, 4000, 10000]

// The close code the server sends at the session's JWT exp (docs/API.md §9.1).
const CLOSE_SESSION_EXPIRED = 4001

export interface MatchSocketOptions {
    onFrame: (frame: WsFrame) => void
    onSessionExpired: () => void
    /** Checked before each reconnect: false once the round is over or the view is gone. */
    shouldReconnect: () => boolean
}

export function useMatchSocket(options: MatchSocketOptions) {
    /** The socket is open, so the poll can slow down. */
    const live = ref(false)
    /** Degraded, not broken: the poll keeps the round moving; only presence stops. */
    const reconnecting = ref(false)

    let socket: MatchSocketHandle | null = null
    // Marks callbacks from a replaced or closed socket as stale, so a late close is not
    // read as a drop.
    let generation = 0
    let attempt = 0
    let heartbeat: number | null = null
    let reconnectTimer: number | null = null

    function stopHeartbeat(): void {
        if (heartbeat !== null) {
            clearInterval(heartbeat)
            heartbeat = null
        }
    }

    /** Close the socket and cancel a pending reconnect. */
    function close(): void {
        if (reconnectTimer !== null) {
            clearTimeout(reconnectTimer)
            reconnectTimer = null
        }
        stopHeartbeat()
        // Bump the generation first, so this socket's late callbacks read as stale.
        generation++
        socket?.close()
        socket = null
        live.value = false
    }

    function scheduleReconnect(id: string): void {
        if (!options.shouldReconnect()) return
        reconnecting.value = true
        const step = Math.min(attempt, RECONNECT_BACKOFF_MS.length - 1)
        attempt += 1
        reconnectTimer = window.setTimeout(() => {
            reconnectTimer = null
            if (options.shouldReconnect()) open(id)
        }, RECONNECT_BACKOFF_MS[step])
    }

    /** Open the match socket, replacing any existing one. */
    function open(id: string): void {
        close()
        const gen = ++generation
        socket = openMatchSocket(id, {
            onOpen: () => {
                if (gen !== generation) return
                reconnecting.value = false
                attempt = 0
                live.value = true
                stopHeartbeat()
                heartbeat = window.setInterval(() => socket?.ping(), PING_MS)
            },
            onClose: (code) => {
                if (gen !== generation) return
                stopHeartbeat()
                live.value = false
                if (code === CLOSE_SESSION_EXPIRED) {
                    options.onSessionExpired()
                    return
                }
                scheduleReconnect(id)
            },
            onError: () => {
                if (gen !== generation) return
                // No detail here; the close event that follows carries the code.
                live.value = false
            },
            onFrame: options.onFrame
        })
    }

    /** Close and forget the backoff, for a new match. */
    function reset(): void {
        close()
        reconnecting.value = false
        attempt = 0
    }

    return { live, reconnecting, open, close, reset }
}
