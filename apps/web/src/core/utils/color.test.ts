import { colord, extend } from 'colord'
import a11yPlugin from 'colord/plugins/a11y'
import { describe, expect, it } from 'vitest'
import { accentSources, inkOn, isDarkColor } from './color'

extend([a11yPlugin])

const PAGE_LIGHT = '#ffffff'
const PAGE_DARK = '#161514'
const PICKS = ['#ffff00', '#00ff00', '#ff5500', '#000080', '#777777', '#ffffff', '#000000', '#00990d', '#e0218a']

describe('accentSources', () => {
    it.each(PICKS)('%s clears the preset bars in both themes', (pick) => {
        const s = accentSources(pick, PAGE_LIGHT, PAGE_DARK)
        expect(colord(s.primaryLight).contrast(PAGE_LIGHT)).toBeGreaterThanOrEqual(3)
        expect(colord(s.primaryDark).contrast(PAGE_DARK)).toBeGreaterThanOrEqual(3)
        expect(colord(s.onPrimaryLight).contrast(s.primaryLight)).toBeGreaterThanOrEqual(4.5)
        expect(colord(s.onPrimaryDark).contrast(s.primaryDark)).toBeGreaterThanOrEqual(4.5)
    })

    it('keeps a pick that already clears the bar', () => {
        expect(accentSources('#006609', PAGE_LIGHT, PAGE_DARK).primaryLight).toBe('#006609')
    })
})

describe('inkOn / isDarkColor', () => {
    it('picks the ink that reads better', () => {
        expect(inkOn('#ffff00')).toBe('#000000')
        expect(inkOn('#000080')).toBe('#ffffff')
    })

    it('treats no colour as white paper', () => {
        expect(isDarkColor(null)).toBe(false)
        expect(isDarkColor('#1c1b1a')).toBe(true)
        expect(isDarkColor('#fcf2c8')).toBe(false)
    })
})
