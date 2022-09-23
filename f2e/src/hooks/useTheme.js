import {watch, ref} from 'vue';

export default function (theme = 'light'){
    const themeRef = ref(theme);
    watch(themeRef, changeTheme);
    const changeTheme = ()=>{
        const themeVal = themeRef.value;
        if(themeVal === 'dark'){
            document.body.setAttribute('arco-theme', themeVal)
        }else{
            document.body.removeAttribute('arco-theme');
        }
    }
    changeTheme();
    return themeRef
}