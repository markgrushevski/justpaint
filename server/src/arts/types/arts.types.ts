import { Buffer } from 'node:buffer'

export type ArtsRelations = Array<'layers' | 'user'>
export type LayersRelations = Array<'art'>

export type CompressedLayer<T extends { dataURL: string | Buffer }> = T & { dataURL: Buffer }

export type DecompressedLayer<T extends { dataURL: string | Buffer }> = T & { dataURL: string }
