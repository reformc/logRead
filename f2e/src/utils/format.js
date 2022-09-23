
const colors = [
    '#f9664c',
    '#ed487b',
    '#b449c2',
    '#885bd2',
    '#8090d3',
    '#65b2fc',
    '#39acf1',
    '#31b8c5',
    '#2ec2b0',
    '#5ac262',
    '#93c55b',
    '#a7da2c',
    '#fada32',
    '#f3c642',
    '#f9a634',
    '#888d92',
];

function getColor (str){
    str = `${str}`; //强制转string
    let num = str.split('').reduce((a,b)=>a+b.charCodeAt(0),0);
    return colors[num % colors.length]
}

function evalJson(json){
    try{
        const obj = JSON.parse(json);
        json = JSON.stringify(obj,null,2);
        console.log(json);
    }catch (e){
        console.err(e);
    }
    return json
}
function parseSquare(code){
    const list = code.split(/(\[[\w\s-\:\$\.\/\+]+\])/g);
    console.log(list);
    return list.map((chunk)=>{
        if(/^\[.*\]$/.test(chunk)){
            console.log(chunk);
            return `<span style="color:${getColor(chunk)}">${chunk}</span>`
        }
        return chunk
    })
}

export default (code)=>{
    try{
        const codes = parseSquare(code);
        code = codes.join('');

    }catch (e){

    }
    return code;
}