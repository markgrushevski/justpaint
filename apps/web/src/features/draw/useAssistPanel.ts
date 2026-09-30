/**
 * The AI assist panel (docs/ASSIST.md): a prompt becomes a batch of ops shown as a
 * ghost in the editor, then accepted on top, accepted as a replacement, or rejected.
 */
import { computed, ref, type ShallowRef } from 'vue'
import type { Editor, Op } from '@justpaint/editor'
import { useAssist } from '@core'

export function useAssistPanel(
    editor: ShallowRef<Editor | null>,
    gated: (reason: string) => Promise<boolean>,
    reportError: (err: unknown, action: string) => void
) {
    const mutation = useAssist()
    const pending = computed(() => mutation.isPending.value)
    const open = ref(false)
    const prompt = ref('')
    // The ghost lives in the editor until Accept; this only flags the accept/reject phase.
    const pendingOps = ref<Op[] | null>(null)
    const note = ref<string | null>(null)

    /** Drop the proposal, ghost included; the document it described is going away. */
    function clear() {
        if (pendingOps.value) editor.value?.rejectOps()
        pendingOps.value = null
        note.value = null
    }

    function toggle() {
        open.value = !open.value
        if (!open.value) clear()
    }

    async function submit() {
        if (!editor.value) return
        const text = prompt.value.trim()
        // Mirrors the submit button's own disabled guard (Enter can reach here too).
        if (!text || pending.value || pendingOps.value) return
        if (!(await gated('Sign in to use assist.'))) return
        // Browser Back can unmount the view while the modal is up.
        const ed = editor.value
        if (!ed) return
        const targetLayerId = ed.getActiveLayerId() || undefined
        mutation.mutate(
            { prompt: text, document: ed.getDocument(), targetLayerId },
            {
                onSuccess: (r) => {
                    editor.value?.previewOps(r.ops)
                    pendingOps.value = r.ops
                    // Shown inline in the panel; a top-center toast would land over it.
                    note.value = r.note ?? null
                },
                onError: (err) => reportError(err, 'use assist')
            }
        )
    }

    // `replace` swaps the whole drawing for the proposal; one Ctrl+Z restores it.
    function accept(mode: 'add' | 'replace') {
        editor.value?.acceptOps(mode)
        pendingOps.value = null
        note.value = null
        prompt.value = ''
    }

    function reject() {
        editor.value?.rejectOps()
        pendingOps.value = null
        note.value = null
    }

    return { open, prompt, pending, pendingOps, note, clear, toggle, submit, accept, reject }
}
