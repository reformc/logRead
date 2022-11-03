import {createApp} from 'vue'
import {Notification} from '@arco-design/web-vue'
import App from './App.vue'
import './style.css'

document.body.setAttribute('arco-theme', 'dark')

const app = createApp(App);
Notification._context = app._context;
// app.use(Notification);
app.mount('#app')