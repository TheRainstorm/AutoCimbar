import { createApp } from 'vue'
import App from './App.vue'
import LogPanel from './LogPanel.vue'
import './style.css'

const kind = new URLSearchParams(window.location.search).get('logs')
if (kind === 'sender' || kind === 'receiver') {
  createApp(LogPanel, { kind, detached: true }).mount('#app')
} else {
  createApp(App).mount('#app')
}
