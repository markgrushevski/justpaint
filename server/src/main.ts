import { NestFactory, Reflector } from '@nestjs/core'
import { ClassSerializerInterceptor, ValidationPipe } from '@nestjs/common'
import { AppModule } from './app.module'
import * as cookieParser from 'cookie-parser'

async function bootstrap() {
    const app = await NestFactory.create(AppModule, {
        cors: { credentials: true, origin: 'http://localhost:7777' },
    })

    app.useGlobalPipes(new ValidationPipe({ transform: true }))
    app.use(cookieParser())

    await app.listen(8888)
}
bootstrap()
