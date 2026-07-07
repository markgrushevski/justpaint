<script lang="ts" setup>
/**
 * A generic, product-agnostic confirmation modal — "are you sure?" before a
 * destructive or irreversible action. Hand-rolled on the ShortcutsDialog /
 * SideMenu pattern (Teleport + Transition + focus-on-open + Esc + Tab-trap +
 * focus-return) rather than @oriui/vue's OriDialog (alpha-2 is uncontrolled:
 * `defaultOpen` + a trigger slot, no `open` prop / `close` emit — it can't be
 * driven by a host `confirmOpen` ref).
 *
 * The parent owns `open` and reacts to `confirm` / `cancel`; this component
 * never mutates the prop. Reused by /draw today and the /play game shell later,
 * so it stays free of any product-specific copy or wiring.
 */
import { nextTick, ref, watch } from 'vue'
import { OriButton } from '@oriui/vue'

const props = defineProps<{
    open: boolean
    title: string
    message?: string
    confirmText?: string
    cancelText?: string
    danger?: boolean
}>()

const emit = defineEmits<{ confirm: []; cancel: [] }>()

const FOCUSABLE = 'a[href], button:not([disabled]), input:not([disabled]), [tabindex]:not([tabindex="-1"])'

// Move focus into the dialog on open (the ShortcutsDialog pattern) — without
// focus inside, the panel-tree Esc handler below would never fire. Focus lands
// on the confirm button (rendered last) so Enter confirms straight away.
const panelRef = ref<HTMLElement | null>(null)
// Whatever had focus when the dialog opened, so we can restore it on close.
const opener = ref<HTMLElement | null>(null)
watch(
    () => props.open,
    async (open) => {
        if (open) {
            opener.value = document.activeElement as HTMLElement | null
            await nextTick()
            const panel = panelRef.value
            const focusables = panel ? Array.from(panel.querySelectorAll<HTMLElement>(FOCUSABLE)) : []
            // Confirm is the last focusable in the footer; fall back to the panel.
            ;(focusables[focusables.length - 1] ?? panel)?.focus()
        } else {
            opener.value?.focus()
        }
    }
)

function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
        emit('cancel')
        return
    }

    if (e.key === 'Tab') {
        const panel = panelRef.value
        if (!panel) return

        const focusables = Array.from(panel.querySelectorAll<HTMLElement>(FOCUSABLE))
        if (focusables.length === 0) {
            // Nothing tabbable inside — keep focus on the panel itself.
            e.preventDefault()
            panel.focus()
            return
        }

        const first = focusables[0]
        const last = focusables[focusables.length - 1]
        const active = document.activeElement
        const outside = !panel.contains(active)

        if (e.shiftKey) {
            if (outside || active === first) {
                e.preventDefault()
                last.focus()
            }
        } else if (active === last) {
            e.preventDefault()
            first.focus()
        }
    }
}
</script>

<template>
    <Teleport to="body">
        <!-- Explicit duration: Vue then uses a timer, not transitionend — deterministic in hidden tabs/tests. -->
        <Transition name="confirm" :duration="200">
            <div v-if="props.open" class="confirm" @keydown="onKeydown">
                <div class="confirm__backdrop" aria-hidden="true" @click="emit('cancel')"></div>

                <div
                    ref="panelRef"
                    class="confirm__panel jp-float"
                    role="dialog"
                    aria-modal="true"
                    :aria-label="props.title"
                    tabindex="-1"
                >
                    <h2 class="confirm__title">{{ props.title }}</h2>
                    <p v-if="props.message" class="confirm__message">{{ props.message }}</p>

                    <div class="confirm__actions">
                        <OriButton
                            :text="props.cancelText ?? 'Cancel'"
                            variant="outline"
                            radius="md"
                            @click="emit('cancel')"
                        />
                        <OriButton
                            :text="props.confirmText ?? 'Confirm'"
                            variant="fill"
                            :color="props.danger ? 'danger' : undefined"
                            radius="md"
                            @click="emit('confirm')"
                        />
                    </div>
                </div>
            </div>
        </Transition>
    </Teleport>
</template>

<style scoped>
.confirm {
    position: fixed;
    inset: 0;
    z-index: 100;

    display: grid;
    place-items: center;

    padding: var(--ori-size-gap_lg, 0.75rem);
}

.confirm__backdrop {
    position: absolute;
    inset: 0;
    background-color: rgb(0 0 0 / 35%);
}

.confirm__panel {
    position: relative;

    display: flex;
    flex-direction: column;
    gap: var(--ori-size-gap_md, 0.5rem);

    width: 100%;
    max-width: 24rem;
    max-height: 100%;
    padding: var(--ori-size-gap_lg, 0.75rem);
    overflow-y: auto;

    color: var(--ori-color-on-surface);
}

.confirm__title {
    margin: 0;
    font-size: var(--ori-font-size_lg, 1.15rem);
    font-weight: 700;
    letter-spacing: -0.01em;
}

.confirm__message {
    margin: 0;
    font-size: var(--ori-font-size_sm, 0.875rem);
    line-height: 1.5;
    opacity: 0.85;
}

/* Cancel then Confirm, right-aligned. */
.confirm__actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--ori-size-gap_sm, 0.25rem);

    margin-top: var(--ori-size-gap_sm, 0.25rem);
}

/* Fade + gentle rise (the toast/menu language, centered). */
.confirm-enter-active,
.confirm-leave-active {
    transition: opacity 180ms ease;
}

.confirm-enter-active .confirm__panel,
.confirm-leave-active .confirm__panel {
    transition: transform 180ms ease;
}

.confirm-enter-from,
.confirm-leave-to {
    opacity: 0;
}

.confirm-enter-from .confirm__panel,
.confirm-leave-to .confirm__panel {
    transform: translateY(0.4rem) scale(0.98);
}
</style>
