/**
 * Открывает печать канваса в новой вкладке браузера.
 * @param {HTMLCanvasElement} canvas
 * @return {boolean}
 * */
export function printCanvas(canvas: HTMLCanvasElement): boolean {
    try {
        const dataURL = canvas.toDataURL()

        const windowContent = `
        <!DOCTYPE html>
        <html lang="en">
            <head><title>Printing</title></head>
            <body><img src="${dataURL}" alt="" style="display: block;"></body>
        </html>
    `

        const printWindow = window.open() as Window
        printWindow.document.open()
        printWindow.document.write(windowContent)
        printWindow.document.close()
        printWindow.onload = () => {
            printWindow.focus()
            printWindow.print()
            printWindow.close()
        }

        return true
    } catch (e) {
        console.error(e)
        throw new Error('Не удалось открыть печать.')
    }
}
