import { Body, Controller, Post, Res, UseGuards } from '@nestjs/common'
import { Response } from 'express'
import { AuthService } from './auth.service'
import { RegisterDto, SignInDto } from './dto/auth.dto'
import { AuthGuard } from './guards/auth.guard'

@Controller('auth')
export class AuthController {
    constructor(private readonly authService: AuthService) {}

    @Post('/register')
    async register(@Res({ passthrough: true }) res: Response, @Body() registerDto: RegisterDto): Promise<boolean> {
        return this.authService.register(res, registerDto)
    }

    @Post('/login')
    async signIn(@Res({ passthrough: true }) res: Response, @Body() signInDto: SignInDto): Promise<boolean> {
        return this.authService.signIn(res, signInDto)
    }

    @UseGuards(AuthGuard)
    @Post('/logout')
    async signOut(@Res({ passthrough: true }) res: Response): Promise<boolean> {
        return this.authService.signOut(res)
    }
}
