import { BadRequestException, Body, Controller, Delete, Get, Param, Post, Req, UseGuards } from '@nestjs/common'
import { SaveArtDto } from './dto/arts.dto'
import { ArtsService } from './arts.service'
import { Request } from '../auth/interfaces/auth.interfaces'
import { AuthGuard } from '../auth/guards/auth.guard'

@UseGuards(AuthGuard)
@Controller('arts')
export class ArtsController {
    constructor(private readonly artsService: ArtsService) {}

    @Get()
    async get(@Req() req: Request) {
        const user = req.user
        if (!user?.sub) throw new BadRequestException()

        return this.artsService.get(user.sub)
    }

    @Post()
    async save(@Req() req: Request, @Body() createArtDto: SaveArtDto) {
        const user = req.user
        if (!user?.sub) throw new BadRequestException()

        await this.artsService.save(user.sub, createArtDto)
        return true
    }

    @Delete('/:id')
    async delete(@Req() req: Request, @Param('id') artId: string) {
        const user = req.user
        if (!user?.sub) throw new BadRequestException()

        await this.artsService.delete(user.sub, artId)
        return true
    }
}
