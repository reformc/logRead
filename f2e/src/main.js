import { createApp } from 'vue'
import { Notification } from '@arco-design/web-vue'
import App from './App.vue'
import './style.css'

const app = createApp(App);
Notification._context = app._context;
// app.use(Notification);
app.mount('#app')
