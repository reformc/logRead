import {watch, ref} from 'vue';

export default function (app){
   const appRef = ref(app);

    return appRef
}