import express from 'express';
//import expressWS from 'express-ws'

const app = express();
//const appWS = expressWS(app)

app.get('/', (req, res) => {
    res.send('Hello World');
});

/* app.post('/api/users', async (req, res) => {
    const result = await db.query('SELECT * FROM users')
    console.log({result})
    res.send(result.rows[0])
}); */

app.listen(8888);
