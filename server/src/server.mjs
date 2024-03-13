import express from 'express'
import expressWS from 'express-ws'

const app = express()
//const appWS = expressWS(app)

app.get('/', (req, res) => {
    res.send('Hello World')
})

app.listen(8888)
