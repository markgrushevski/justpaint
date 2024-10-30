import { IsNotEmpty, IsString, Length } from 'class-validator'

export class RegisterDto {
    @Length(3, 20)
    nickname: string
    @Length(6, 32)
    password: string
}

export class SignInDto {
    @Length(3, 20, { message: 'User not found' })
    nickname: string
    @Length(6, 32, { message: 'User not found' })
    password: string
}
