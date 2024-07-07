export function getUId() {
    return Date.now() + Number(Math.random().toFixed(6)) * 1000000;
}
