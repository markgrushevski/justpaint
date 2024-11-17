import type { Art, ArtLayer } from '@core'
import { createImage, getCanvasDataURL } from '@modules/canvas'

export async function getCompositedArts(arts: Art[]): Promise<Art[]> {
    console.log({ toComposite: arts })

    // const arts = [..._arts]
    // arts[0] = { ...arts[0], layers: [...arts[0].layers, arts[3].layers[0]] }
    // console.log({ layers: arts[0].layers })

    return Promise.all(
        arts.map(async (art) => {
            return {
                ...art,
                dataURL: await getCompositedArtLayers(art)
            }
        })
    )
}

export async function getCompositedArtLayers(art: Art): Promise<string> {
    const canvas = document.createElement('canvas')
    const ctx = canvas.getContext('2d')!

    const layersImages = await layersToImages(art.layers)

    const maxLayerImageWidth = layersImages.toSorted((image1, image2) => image2.width - image1.width)[0].width
    const maxLayerImageHeight = layersImages.toSorted((image1, image2) => image2.height - image1.height)[0].height

    canvas.width = maxLayerImageWidth
    canvas.height = maxLayerImageHeight

    for (const layerImage of layersImages) {
        const layerDx = (maxLayerImageWidth - layerImage.width) / 2
        const layerDy = (maxLayerImageHeight - layerImage.height) / 2

        ctx.drawImage(layerImage, layerDx, layerDy, layerImage.width, layerImage.height)
    }

    return getCanvasDataURL(canvas)
}

export async function layersToImages(layers: ArtLayer[]): Promise<HTMLImageElement[]> {
    return Promise.all(
        layers.map(async (layer) => {
            return createImage(layer.dataURL)
        })
    )
}
