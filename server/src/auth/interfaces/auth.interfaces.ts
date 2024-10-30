import { JwtPayload } from 'jsonwebtoken'
import type { Request as ExpressRequest } from 'express'

export interface JWTPayload extends JwtPayload {
    sub?: string
    username?: string
}

export interface Request extends ExpressRequest {
    user?: JWTPayload
}
