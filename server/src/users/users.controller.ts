import {
    BadRequestException,
    Body,
    Controller,
    Get,
    NotFoundException,
    Param,
    Post,
    Req,
    UseGuards
} from '@nestjs/common'
import { UsersService } from './users.service'
import { Request } from '../auth/interfaces/auth.interfaces'
import { AuthGuard } from '../auth/guards/auth.guard'

@UseGuards(AuthGuard)
@Controller('users')
export class UsersController {
    constructor(private readonly usersService: UsersService) {}

    @Get('/nickname')
    async getNickname(@Req() req: Request) {
        return this.usersService.checkNickname(req.user?.username ?? '')
    }

    /*@Get()
  async getAll() {
    return this.usersService.getAll();
  }*/

    /*@Get('/:nickname')
  async getByNickname(@Param('nickname') nickname: string) {
    return this.usersService.getByNickname(nickname);
  }*/
}
