import type { PanTool } from '../types'

/**
 * Hand — pans the view on a primary pointer drag (mouse left-drag or a
 * single-finger touch drag, which finally gives touch users a way to pan; the
 * middle-button drag keeps working with every tool). The pan mechanics
 * (clamp-free `panBy`, the stage transform, pointer capture/abandon hardening,
 * the grab/grabbing cursor) live in the Editor's shared pan path — the same
 * code the middle-button drag uses.
 */
export const handTool: PanTool = {
    kind: 'pan',
    id: 'hand'
}
