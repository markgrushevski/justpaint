import Pool from 'pg-pool'

const DB_URL = new URL(process.env.DATABASE_URL)

const pool = new Pool({
    user: DB_URL.username ,
    password: DB_URL.password,
    host: DB_URL.hostname,
    port: +DB_URL.port,
    database: DB_URL.pathname.split('/')[1],
    ssl: true
})

export class DB {
    /**
     * @param {import("pg").QueryArrayConfig<any>} text
     * @param {any} params
     */
    static async query(text, params) {
        const startTime = Date.now();
        const res = await pool.query(text, params);
        const endTime = Date.now();
        const duration = endTime - startTime;
        console.log('executed query', { text, duration, rows: res.rowCount });
        return res;
    }
}
