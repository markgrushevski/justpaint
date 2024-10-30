import { ClassSerializerInterceptor, Module } from '@nestjs/common'
import { AppController } from './app.controller'
import { AppService } from './app.service'
import { TypeOrmModule } from '@nestjs/typeorm'
import { ConfigModule, ConfigService } from '@nestjs/config'
import { SnakeNamingStrategy } from 'typeorm-naming-strategies'
import { AuthModule } from './auth/auth.module'
import { ArtsModule } from './arts/arts.module'
import { APP_INTERCEPTOR } from '@nestjs/core'

@Module({
    imports: [
        ConfigModule.forRoot({ isGlobal: true, cache: true }),
        TypeOrmModule.forRootAsync({
            imports: [ConfigModule],
            inject: [ConfigService],
            useFactory: (configService: ConfigService) => {
                const PG_URL = configService.get<string>('POSTGRES_URL') ?? ''
                const DB_URL = new URL(PG_URL)
                return {
                    type: 'postgres',
                    host: DB_URL.hostname,
                    port: +DB_URL.port,
                    username: DB_URL.username,
                    password: DB_URL.password,
                    database: DB_URL.pathname.split('/')[1],
                    ssl: true,
                    namingStrategy: new SnakeNamingStrategy(),
                    relationLoadStrategy: 'query',
                    autoLoadEntities: true,
                    synchronize: true,
                    logging: true,
                }
            },
        }),
        AuthModule,
        ArtsModule,
    ],
    controllers: [AppController],
    providers: [
        {
            provide: APP_INTERCEPTOR,
            useClass: ClassSerializerInterceptor,
        },
        AppService,
    ],
})
export class AppModule {}
