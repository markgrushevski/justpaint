import { CanActivate, ExecutionContext, Injectable, UnauthorizedException } from '@nestjs/common'
import { JwtService } from '@nestjs/jwt'
import { ConfigService } from '@nestjs/config'
import { Request } from '../interfaces/auth.interfaces'

@Injectable()
export class AuthGuard implements CanActivate {
    constructor(
        private readonly jwtService: JwtService,
        private readonly configService: ConfigService,
    ) {}

    async canActivate(context: ExecutionContext): Promise<boolean> {
        const request = context.switchToHttp().getRequest() as Request
        const token = this.extractTokenFromCookie(request)
        if (!token) throw new UnauthorizedException()

        try {
            const secret = this.configService.get<string>('JWT_SECRET') ?? ''
            request.user = await this.jwtService.verifyAsync(token, { secret })
        } catch {
            throw new UnauthorizedException()
        }
        return true
    }

    private extractTokenFromHeader(request: Request): string | undefined {
        const [type, token] = request.headers.authorization?.split(' ') ?? []
        return type === 'Bearer' ? token : undefined
    }

    private extractTokenFromCookie(request: Request): string | undefined {
        return request.cookies['jwt']
    }
}
