import Emitter from './emitter'
import debounce from "./debounce.js";
import throttle from "./throttle.js";

const maxLogLen = 4 * 1024 * 1024;

/**
 * 字符串转uint8Array
 * @param str
 * @returns {Uint8Array}
 */
function str2Byte(str){
    const bytes = new Array();
    let len, c;
    len = str.length;
    for (let i = 0; i < len; i++) {
        c = str.charCodeAt(i);
        if (c >= 0x010000 && c <= 0x10FFFF) { // 四个字节
            bytes.push(((c >> 18) & 0x07) | 0xF0);
            bytes.push(((c >> 12) & 0x3F) | 0x80);
            bytes.push(((c >> 6) & 0x3F) | 0x80);
            bytes.push((c & 0x3F) | 0x80);
        } else if (c >= 0x000800 && c <= 0x00FFFF) { // 三个字节
            bytes.push(((c >> 12) & 0x0F) | 0xE0);
            bytes.push(((c >> 6) & 0x3F) | 0x80);
            bytes.push((c & 0x3F) | 0x80);
        } else if (c >= 0x000080 && c <= 0x0007FF) { //二个字节
            bytes.push(((c >> 6) & 0x1F) | 0xC0);
            bytes.push((c & 0x3F) | 0x80);
        } else {
            bytes.push(c & 0xFF); //1个字节
        }
    }
    return new Uint8Array(bytes);
}

/**
 * 00001000
 *
 * arrayBuff 转 string
 * @param buf
 * @returns {*[]}
 */
function byte2Str(buf){
    try{
        buf = buf.slice(0, maxLogLen); //防止长度越界
        let bytes = new Uint8Array(buf);
        return new TextDecoder().decode(bytes);
    }catch (e){
        console.error('日志解析出错',e,new Uint8Array(buf));
    }
    return '';
}


class Ws extends Emitter{
    constructor(url, protocols) {
        super();
        this.pool = [];
        this.heartBeat = debounce(()=>{
            this.send("0");
            if(this.isOpen) this.heartBeat();
        }, 30000, false);

        this.onMessage = throttle(()=>{
            const msg = this.pool.slice();
            this.emit('message', msg);
            this.pool= [];
            this.heartBeat();
        }, 200, this);
    }

    /**
     * 0 连接中， 1 连接中， 2 关闭中 3 关闭了
     * @returns {number}
     */
    get status (){
        return this._ws.readyState;
    }
    get isOpen(){
        return this.status === 1;
    }

    open(url, protocols){
        return new Promise((resolve,reject)=>{
            let {protocol, host} = location;
            protocol = /https/.test(protocol)?'wss':'ws'
            url = url || `${protocol}://${host}/readlog/wsapi`;
            this._ws = new WebSocket(url, protocols);
            this._ws.binaryType = 'arraybuffer'
            this._ws.onopen = (evt)=>{
                this.emit('open',evt);
                this.heartBeat();
                resolve();
            }
            this._ws.onclose = (evt)=>{
                this.emit('close', evt);
            }
            this._ws.onmessage = (evt)=>{
                const msg = byte2Str(evt.data);
                this.pool.push(msg);
                if(msg.includes('->message send over<-')){
                    this.emit('message-end',msg);
                }
                this.onMessage();
            }
            this._ws.onerror = (evt)=>{
                this.emit('error', evt);
            }
            this.once('error', reject);
        })

    }

    send(data){
        return new Promise((resolve, reject)=>{
            if(!this.isOpen){
                this.on('open', ()=>{
                    this.send(data)
                })
                return;
            }
            try {
                data = JSON.stringify(data);
                const bytes = str2Byte(data);
                this._ws.send(bytes);
                this.heartBeat();
            }catch (e){
                reject(e);
            }
            this.on('message-end',()=>{
                console.log('send end');
                resolve();
            });
        })
    }
    close(){
        return new Promise((resolve, reject)=>{
            this.once('error', reject)
            this.once('close', resolve)
            this._ws.close();
        })
    }
}

export default new Ws();