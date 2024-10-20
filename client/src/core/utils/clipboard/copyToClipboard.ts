/**
 * Копирует переданный текст в буфер обмена.
 * Работает только с https протоколом.
 * @param {'text' | 'image'} type
 * @param {string | Blob} value
 * @throws {Error} Если не удалось скопировать текст в буфер обмена.
 * @return {Promise<boolean>} Промис, который резолвится после успешного копирования текста в буфер обмена.
 */
export async function copyToClipboard(type: 'text' | 'image', value: string | Blob): Promise<boolean> {
    try {
        if (type === 'text' && typeof value === 'string') {
            await navigator.clipboard.writeText(value)
        } else if (type === 'image' && typeof value === 'object') {
            await navigator.clipboard.write([new ClipboardItem({ [value.type]: value })])
        } else {
            return false
        }

        return true
    } catch (e) {
        console.error(e)
        throw new Error('Не удалось скопировать текст.')
    }
}
