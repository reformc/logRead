export default function throttle(cb, wait, ctx) {
    let timer, args, last=0;

    function trigger(){
        last = Date.now();
        cb.apply(this, args);
        timer =  args = null;
    }

    return function () {
        if (timer) return;
        args = arguments; // 触发时的参数与调用的保持一致

        const delta = Date.now()- last;
        if(delta > wait)  trigger.apply(ctx || this);
        else timer = setTimeout(trigger.bind(ctx || this), wait- delta);
    }
}
