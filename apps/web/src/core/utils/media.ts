import { onScopeDispose, readonly, ref } from 'vue'
import type { Ref } from 'vue'

/** Whether a media query matches, kept live; the listener goes with the calling scope. */
export function useMediaQuery(query: string): Readonly<Ref<boolean>> {
    const list = window.matchMedia(query)
    const matches = ref(list.matches)
    const onChange = (e: MediaQueryListEvent): void => {
        matches.value = e.matches
    }
    list.addEventListener('change', onChange)
    onScopeDispose(() => list.removeEventListener('change', onChange))
    return readonly(matches)
}
