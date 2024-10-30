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
    createdAt?: Date
    updatedAt?: Date
}

export interface ArtLayer {
    name: string
    dataURL: string
}
