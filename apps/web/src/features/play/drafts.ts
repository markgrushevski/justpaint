/**
 * The duel drawing kept in sessionStorage by match id while the round runs, so a reload
 * or a return to /play resumes it. Best effort: storage can be full, blocked or absent.
 */
import type { Document } from '@justpaint/editor'

const PREFIX = 'jp:duel-draft:'

function storageOr(storage: Storage | undefined): Storage | null {
    try {
        return storage ?? window.sessionStorage
    } catch {
        return null
    }
}

export function saveDraft(matchId: string, doc: Document, storage?: Storage): void {
    try {
        storageOr(storage)?.setItem(PREFIX + matchId, JSON.stringify(doc))
    } catch {
        // full or blocked: the round goes on without a draft
    }
}

/** The kept drawing, or null; `width` is the canvas the duel requires. */
export function loadDraft(matchId: string, width: number, storage?: Storage): Document | null {
    try {
        const raw = storageOr(storage)?.getItem(PREFIX + matchId)
        if (!raw) return null
        const doc = JSON.parse(raw) as Document
        return doc.width === width && doc.height === width && Array.isArray(doc.layers) ? doc : null
    } catch {
        return null
    }
}

/** Drops every draft but `keep`'s: a player has one match in play, so the others are over. */
export function clearDrafts(keep: string | null = null, storage?: Storage): void {
    try {
        const store = storageOr(storage)
        if (!store) return
        for (let i = store.length - 1; i >= 0; i--) {
            const key = store.key(i)
            if (key?.startsWith(PREFIX) && key !== PREFIX + keep) store.removeItem(key)
        }
    } catch {
        // blocked: nothing was kept either
    }
}
