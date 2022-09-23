import Emitter from './emitter'
import debounce from "./debounce.js";
import throttle from "./throttle.js";

class Ws extends Emitter{
    constructor(url, protocols) {
        super();
        this.pool = [];
        this.heartBeat = debounce(()=>{
            this.send("0");
            if(this.isOpen) this.heartBeat();
            console.log('send', this.isOpen);
        }, 30000, false, this);

        this.onMessage = throttle(()=>{
            const msg = this.pool.slice();
            this.emit('message', msg);
            this.pool= [];
            this.heartBeat();
        }, 100, this);
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
            url = url || `ws://${window.location.host}/readlog/wsapi`;
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
                console.log(typeof  evt.data);
                this.pool.push(evt.data);
                this.onMessage();
            }
            this._ws.onerror = (evt)=>{
                this.emit('error', evt);
            }
            this.once('error', reject);
        })

    }

    send(data){
        if(!this.isOpen){
            this.on('open', ()=>{
                this.send(data)
            })
            return;
        }
        try {
            data = JSON.stringify(data);
            this._ws.send(data);
            this.heartBeat();
        }catch (e){
            console.log('error', e);
        }
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