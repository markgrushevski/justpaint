export interface Register {
    nickname: string
    password: string
}

export interface Login {
    nickname: string
    password: string
}

export interface Art {
    id?: string
    name: string
    layers: ArtLayer[]
    dataURL?: string
    createdAt?: Date
    updatedAt?: Date
}

export interface ArtLayer {
    id?: string
    artId?: string
    name: string
    dataURL: string
}

export interface SavedArtIds {
    artId: string
    layerIds: string[]
}
