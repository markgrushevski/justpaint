import express from 'express';
import cors from 'cors';
import expressWS from 'express-ws';
import { expressjwt as jwt } from 'express-jwt';
import { DB } from './db/index.js';

//process.env['NODE_TLS_REJECT_UNAUTHORIZED'] = '0';

const app = express();
app.use(cors());
app.use(express.json());
const appWS = expressWS(app);

app.use(
    jwt({
        secret: process.env.JWT_SECRET,
        algorithms: ['HS256']
    }).unless({ path: ['/api/login'] })
);

app.post('/api/login', (req, res) => {
    console.log('LOGIN', req.auth);
    if (
        !req.body ||
        !req.body.nickname ||
        !req.body.password ||
        typeof req.body.nickname !== 'string' ||
        typeof req.body.password !== 'string'
    ) {
        res.status(400).send({ error: 'Bad request' });
        return;
    }
    res.send(req.body);
});

app.post('/api/logout', (req, res) => {
    res.send('Hello World!');
});

app.get('/api/works', async (req, res) => {
    const result = await DB.query('SELECT * FROM works');
    res.send(result.rows);
});

app.post('/api/works', async (req, res) => {
    if (!req.body) {
        res.status(400).send({ error: 'Bad request' });
        return;
    }
    const value = req.body[0].value;
    const query = 'INSERT INTO works(value) VALUES($1)';
    const values = [value];
    const result = await DB.query(query, values);
    res.send(result.rows[0]);
});

app.listen(8888);
