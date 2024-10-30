import { ConflictException, Injectable, UnauthorizedException } from '@nestjs/common'
import { JwtService } from '@nestjs/jwt'
import { Response } from 'express'
import { RegisterDto, SignInDto } from './dto/auth.dto'
import { UsersService } from '../users/users.service'
import { JWTPayload } from './interfaces/auth.interfaces'

@Injectable()
export class AuthService {
    constructor(
        private readonly usersService: UsersService,
        private readonly jwtService: JwtService,
    ) {}

    async register(res: Response, registerDto: RegisterDto): Promise<boolean> {
        const { nickname, password } = registerDto

        const existedUser = await this.usersService.getByNickname(nickname)
        if (existedUser) {
            throw new ConflictException(`User ${nickname} already exists`)
        }

        const userId = await this.usersService.create(nickname, password)

        const { accessToken } = await this.generateToken(userId, nickname)

        this.setSessionIdCookie(res, accessToken)

        return true
    }

    async signIn(res: Response, signInDto: SignInDto): Promise<boolean> {
        const { nickname, password } = signInDto

        const user = await this.usersService.getByNickname(nickname)
        if (!user) {
            throw new UnauthorizedException(`User ${nickname} not found`)
        } else if (user.password !== password) {
            throw new UnauthorizedException(`Wrong password`)
        }

        const { accessToken } = await this.generateToken(user.id, user.nickname)

        this.setSessionIdCookie(res, accessToken)

        return true
    }

    async signOut(res: Response): Promise<boolean> {
        this.clearSessionIdCookie(res)
        return true
    }

    private async generateToken(userId: string, nickname: string): Promise<{ accessToken: string }> {
        const payload: JWTPayload = { sub: userId, username: nickname }
        const accessToken = await this.jwtService.signAsync(payload)
        return { accessToken }
    }

    private setSessionIdCookie(res: Response, accessToken: string): boolean {
        res.cookie('jwt', accessToken, {
            maxAge: 20 * 60 * 1000,
            httpOnly: true,
            secure: false,
        })

        return true
    }

    private clearSessionIdCookie(res: Response) {
        res.clearCookie('jwt', {
            httpOnly: true,
            secure: false,
        })
        return true
    }
}
