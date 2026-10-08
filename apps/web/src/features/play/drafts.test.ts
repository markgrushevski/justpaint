import { describe, expect, it } from 'vitest'
import type { Document } from '@justpaint/editor'
import { clearDrafts, loadDraft, saveDraft } from './drafts'

/** The slice of Storage the drafts use. */
function memoryStorage(): Storage {
    const map = new Map<string, string>()
    return {
        get length() {
            return map.size
        },
        key: (i: number) => [...map.keys()][i] ?? null,
        getItem: (k: string) => map.get(k) ?? null,
        setItem: (k: string, v: string) => {
            map.set(k, v)
        },
        removeItem: (k: string) => {
            map.delete(k)
        },
        clear: () => map.clear()
    }
}

function doc(width = 1080): Document {
    return { version: 1, width, height: width, background: '#ffffff', layers: [] }
}

describe('drafts', () => {
    it('round-trips by match id', () => {
        const store = memoryStorage()
        saveDraft('m1', doc(), store)
        expect(loadDraft('m1', 1080, store)).toEqual(doc())
        expect(loadDraft('m2', 1080, store)).toBeNull()
    })

    it('refuses a draft for another canvas or a broken one', () => {
        const store = memoryStorage()
        saveDraft('m1', doc(500), store)
        expect(loadDraft('m1', 1080, store)).toBeNull()
        store.setItem('jp:duel-draft:m2', '{not json')
        expect(loadDraft('m2', 1080, store)).toBeNull()
    })

    it('clears every draft but the one kept, and leaves other keys alone', () => {
        const store = memoryStorage()
        saveDraft('m1', doc(), store)
        saveDraft('m2', doc(), store)
        store.setItem('other', 'x')
        clearDrafts('m2', store)
        expect(loadDraft('m1', 1080, store)).toBeNull()
        expect(loadDraft('m2', 1080, store)).not.toBeNull()
        clearDrafts(null, store)
        expect(loadDraft('m2', 1080, store)).toBeNull()
        expect(store.getItem('other')).toBe('x')
    })

    it('a full storage is not an error', () => {
        const store = memoryStorage()
        store.setItem = () => {
            throw new Error('QuotaExceededError')
        }
        expect(() => saveDraft('m1', doc(), store)).not.toThrow()
    })
})
