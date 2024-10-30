import { ArrayMinSize, IsBase64, IsUUID, Length, ValidateIf } from 'class-validator'

export class SaveArtDto {
    @ValidateIf((self) => typeof self.id === 'string')
    @IsUUID(4)
    id?: string
    @Length(1, 64)
    name: string
    @ArrayMinSize(1)
    layers: SaveArtLayerDto[]
}

export class SaveArtLayerDto {
    @ValidateIf((self) => typeof self.id === 'string')
    @IsUUID(4)
    id?: string
    @ValidateIf((self) => typeof self.artId === 'string')
    @IsUUID(4)
    artId?: string
    @Length(1, 64)
    name: string
    @IsBase64()
    dataURL: string
}
