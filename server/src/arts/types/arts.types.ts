export type ArtsRelations = Array<'layers' | 'user'>
export type LayersRelations = Array<'art'>

export type CompressedLayer<T extends object> = T & { dataURL: ArrayBuffer }

export type DecompressedLayer<T extends object> = T & { dataURL: string }
