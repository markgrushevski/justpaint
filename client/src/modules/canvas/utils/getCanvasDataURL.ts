/**
 * Получение DataURL канваса.
 * @param {HTMLCanvasElement} canvas Элемент канваса.
 * @return {string}
 */
export function getCanvasDataURL(canvas: HTMLCanvasElement): string {
    return canvas?.toDataURL('image/png', 1) ?? ''
}
