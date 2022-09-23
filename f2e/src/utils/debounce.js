export default function (cb, wait, immediate, ctx) {
    let timer, last, args;
    function trigger() {
        let time = Date.now() - last;
        if (time > 0 && time < wait) {
            timer = setTimeout(() => trigger.apply(this), wait - time);
        } else {
            timer = last = null;
            if(!immediate) cb.apply(this, args);
            args = null;
        }
    }
    return function () {
        last = Date.now();
        args = [...arguments]; //多次传入的args 不一样 取最近调用的那次
        if(timer) return;
        timer = setTimeout(()=> trigger.apply(ctx || this), wait);
        if(immediate) cb.apply(this, args);
    }
}
