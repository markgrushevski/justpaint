/**
 * Преобразовывает канвас в блоб.
 * @param {HTMLCanvasElement} canvas Элемент канваса.
 * @throws {Error} Если не удалось преобразовать канвас в блоб.
 * @return {Promise<Blob>} Промис, который резолвится после успешного преобразования канваса в блоб.
 */
export async function getCanvasBlob(canvas: HTMLCanvasElement): Promise<Blob> {
    return new Promise((resolve, reject) => {
        canvas.toBlob((blob) => {
            if (blob) resolve(blob)
            else reject(new Error('Не удалось преобразовать канвас.'))
        })
    })
}
