import { createApp } from 'vue';
import { createPinia } from 'pinia';
import App from './App.vue';
import router from './router/index.js';
import { createUserManager } from './auth/oidc.js';
import './styles.css';

createUserManager();

const app = createApp(App);
app.use(createPinia());
app.use(router);
app.mount('#app');
