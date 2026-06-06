<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import {
  AppService,
  ConfigService,
  DecoderService,
  EncoderService,
  defaultConfig,
  onEvent,
  type ReceiverMetrics,
  type ReceiverSession,
  type ScreenInfo,
  type SelectedFile,
  type SenderSession,
  type TaskState,
  type TransferConfig,
} from './runtime/api'

const config = reactive<TransferConfig>({ ...defaultConfig })
const screens = ref<ScreenInfo[]>([])
const selectedFile = ref<SelectedFile | null>(null)
const sender = ref<SenderSession | null>(null)
const receiver = ref<ReceiverSession | null>(null)
const senderState = ref<TaskState>('idle')
const receiverState = ref<TaskState>('idle')
const advancedOpen = ref(false)
const senderLogs = ref<string[]>([])
const receiverLogs = ref<string[]>([])
const metrics = reactive<ReceiverMetrics>({
  sessionId: '',
  state: 'idle',
  progress: 0,
  speedKBps: 0,
  fps: 0,
  etaSeconds: 0,
  rank: 0,
  blocks: 0,
  output: '',
  updatedAt: '',
})

const canStartSend = computed(() => !!selectedFile.value && senderState.value !== 'running')
const canControlSender = computed(() => !!sender.value)
const canControlReceiver = computed(() => !!receiver.value)
const etaText = computed(() => {
  const seconds = metrics.etaSeconds
  if (!seconds) return '--'
  if (seconds < 60) return `${seconds}s`
  const m = Math.floor(seconds / 60)
  const s = seconds % 60
  return `${m}m ${s}s`
})
const ringStyle = computed(() => {
  const value = Math.max(0, Math.min(100, metrics.progress || 0))
  return { background: `conic-gradient(#38bdf8 ${value * 3.6}deg, rgba(31,41,55,.9) 0deg)` }
})

function pushLog(target: typeof senderLogs | typeof receiverLogs, message: string) {
  target.value = [...target.value.slice(-80), message]
}

async function loadInitial() {
  Object.assign(config, await ConfigService.getConfig())
  screens.value = await AppService.listScreens()
}

async function chooseFile() {
  const file = await AppService.selectFileToSend()
  if (!file.path) return
  selectedFile.value = file
  sender.value = await EncoderService.prepareSend(file.path, { ...config })
  senderState.value = sender.value.state
}

async function chooseOutput() {
  const path = await AppService.selectOutputDirectory()
  if (path) config.output = path
}

async function startSender() {
  if (!selectedFile.value) return
  await ConfigService.saveConfig({ ...config })
  if (!sender.value || senderState.value !== 'paused') {
    sender.value = await EncoderService.prepareSend(selectedFile.value.path, { ...config })
  }
  senderState.value = 'running'
  await EncoderService.startSend(sender.value.id)
}

async function pauseSender() {
  if (!sender.value) return
  senderState.value = 'paused'
  await EncoderService.pauseSend(sender.value.id)
}

async function resumeSender() {
  if (!sender.value) return
  senderState.value = 'running'
  await EncoderService.resumeSend(sender.value.id)
}

async function stopSender() {
  if (!sender.value) return
  senderState.value = 'stopped'
  await EncoderService.stopSend(sender.value.id)
}

async function startReceiver() {
  await ConfigService.saveConfig({ ...config })
  if (!receiver.value || receiverState.value === 'done' || receiverState.value === 'stopped') {
    receiver.value = await DecoderService.prepareReceive({ ...config })
  }
  const wasPaused = receiverState.value === 'paused'
  receiverState.value = 'running'
  if (wasPaused) {
    await DecoderService.resumeReceive(receiver.value.id)
  } else {
    await DecoderService.startReceive(receiver.value.id)
  }
}

async function pauseReceiver() {
  if (!receiver.value) return
  receiverState.value = 'paused'
  await DecoderService.pauseReceive(receiver.value.id)
}

async function resumeReceiver() {
  if (!receiver.value) return
  receiverState.value = 'running'
  await DecoderService.resumeReceive(receiver.value.id)
}

async function stopReceiver() {
  if (!receiver.value) return
  receiverState.value = 'stopped'
  await DecoderService.stopReceive(receiver.value.id)
}

onMounted(() => {
  void loadInitial()
  onEvent<SenderSession>('sender:state', (payload) => {
    senderState.value = payload.state
    sender.value = payload
  })
  onEvent<{ message: string }>('sender:log', (payload) => pushLog(senderLogs, payload.message))
  onEvent<{ error: string }>('sender:error', (payload) => pushLog(senderLogs, `ERROR: ${payload.error}`))
  onEvent<{ fileName: string; md5: string }>('sender:done', (payload) =>
    pushLog(senderLogs, `DONE: ${payload.fileName} md5=${payload.md5}`),
  )
  onEvent<ReceiverSession>('receiver:state', (payload) => {
    receiverState.value = payload.state
    receiver.value = payload
  })
  onEvent<{ message: string }>('receiver:log', (payload) => pushLog(receiverLogs, payload.message))
  onEvent<ReceiverMetrics>('receiver:metrics', (payload) => Object.assign(metrics, payload))
  onEvent<{ error: string }>('receiver:error', (payload) => pushLog(receiverLogs, `ERROR: ${payload.error}`))
  onEvent<{ output: string; md5: string }>('receiver:done', (payload) =>
    pushLog(receiverLogs, `DONE: ${payload.output} md5=${payload.md5}`),
  )
})
</script>

<template>
  <main class="h-screen overflow-auto bg-gray-950 text-gray-100">
    <div class="flex min-h-full w-full flex-col gap-3 px-4 py-3">
      <section class="rounded-xl border border-white/10 bg-gray-900/80 p-3 shadow-lg backdrop-blur-xl">
        <div class="grid gap-3 md:grid-cols-[110px_1fr_auto]">
          <label class="block">
            <span class="text-xs text-gray-400">RQ</span>
            <input v-model.number="config.rq" type="number" min="1" class="mt-1 h-9 w-full rounded-lg border border-white/10 bg-gray-800 px-3 text-sm text-gray-100 outline-none focus:border-sky-400" />
          </label>
          <label class="block">
            <span class="text-xs text-gray-400">Screen</span>
            <select v-model.number="config.screen" class="mt-1 h-9 w-full rounded-lg border border-white/10 bg-gray-800 px-3 text-sm text-gray-100 outline-none focus:border-sky-400">
              <option v-for="screen in screens" :key="screen.index" :value="screen.index">{{ screen.label }}</option>
            </select>
          </label>
          <button class="self-end rounded-lg border border-white/10 bg-gray-800 px-4 py-2 text-sm text-gray-100 shadow-lg transition-all hover:scale-105 hover:bg-gray-700" @click="advancedOpen = !advancedOpen">
            Advanced
          </button>
        </div>

        <div class="grid overflow-hidden transition-all duration-300 ease-out" :class="advancedOpen ? 'grid-rows-[1fr] opacity-100' : 'grid-rows-[0fr] opacity-0'">
          <div class="min-h-0">
            <div class="mt-3 grid gap-3 md:grid-cols-5">
              <label class="block">
                <span class="text-xs text-gray-400">Cell (-c)</span>
                <input v-model="config.cell" class="mt-1 h-9 w-full rounded-lg border border-white/10 bg-gray-800 px-3 text-sm text-gray-100 outline-none focus:border-sky-400" />
              </label>
              <label class="block">
                <span class="text-xs text-gray-400">ECC</span>
                <input v-model.number="config.ecc" type="number" min="0" max="100" class="mt-1 h-9 w-full rounded-lg border border-white/10 bg-gray-800 px-3 text-sm text-gray-100 outline-none focus:border-sky-400" />
              </label>
              <label class="block">
                <span class="text-xs text-gray-400">Packets (-p)</span>
                <input v-model.number="config.packets" type="number" min="1" class="mt-1 h-9 w-full rounded-lg border border-white/10 bg-gray-800 px-3 text-sm text-gray-100 outline-none focus:border-sky-400" />
              </label>
              <label class="block">
                <span class="text-xs text-gray-400">B</span>
                <input v-model.number="config.scale" type="number" min="1" class="mt-1 h-9 w-full rounded-lg border border-white/10 bg-gray-800 px-3 text-sm text-gray-100 outline-none focus:border-sky-400" />
              </label>
              <label class="block">
                <span class="text-xs text-gray-400">X:Y</span>
                <input v-model="config.position" class="mt-1 h-9 w-full rounded-lg border border-white/10 bg-gray-800 px-3 text-sm text-gray-100 outline-none focus:border-sky-400" />
              </label>
              <label class="block">
                <span class="text-xs text-gray-400">FPS</span>
                <input v-model.number="config.fps" type="number" min="1" class="mt-1 h-9 w-full rounded-lg border border-white/10 bg-gray-800 px-3 text-sm text-gray-100 outline-none focus:border-sky-400" />
              </label>
              <label class="block">
                <span class="text-xs text-gray-400">Backend</span>
                <select v-model="config.backend" class="mt-1 h-9 w-full rounded-lg border border-white/10 bg-gray-800 px-3 text-sm text-gray-100 outline-none focus:border-sky-400">
                  <option value="symbols">symbols</option>
                  <option value="qr">qr</option>
                </select>
              </label>
              <label class="block">
                <span class="text-xs text-gray-400">Symbols (-s)</span>
                <input v-model="config.symbols" placeholder="built-in" class="mt-1 h-9 w-full rounded-lg border border-white/10 bg-gray-800 px-3 text-sm text-gray-100 outline-none focus:border-sky-400" />
              </label>
              <label class="block">
                <span class="text-xs text-gray-400">Workers</span>
                <input v-model.number="config.decodeWorkers" type="number" min="0" class="mt-1 h-9 w-full rounded-lg border border-white/10 bg-gray-800 px-3 text-sm text-gray-100 outline-none focus:border-sky-400" />
              </label>
              <label class="flex items-center gap-2 self-end rounded-lg border border-white/10 bg-gray-800 px-3 py-2 text-sm text-gray-100">
                <input v-model="config.noZstd" type="checkbox" class="h-4 w-4 accent-sky-500" />
                <span>No zstd</span>
              </label>
            </div>
          </div>
        </div>
      </section>

      <section class="grid grid-cols-1 gap-3 lg:grid-cols-[1fr_1fr]">
        <div class="rounded-xl border border-white/10 bg-gray-900/75 p-4 shadow-glow backdrop-blur-xl">
          <div class="mb-3 flex items-center justify-between">
            <div>
              <h2 class="text-lg font-semibold text-white">Sender</h2>
              <p class="text-xs text-gray-400">Native high-fps render window</p>
            </div>
            <span class="rounded-lg bg-gray-800 px-2 py-1 text-xs uppercase text-sky-200">{{ senderState }}</span>
          </div>

          <button class="group flex h-28 w-full flex-col items-center justify-center rounded-xl border border-dashed border-sky-300/30 bg-gray-800/70 p-4 text-center shadow-lg transition-all hover:scale-[1.01] hover:border-sky-300/70 hover:bg-gray-800" @click="chooseFile">
            <div class="mb-2 grid h-10 w-10 place-items-center rounded-lg bg-sky-400/15 text-xl text-sky-200 transition-all group-hover:scale-105">↑</div>
            <div class="text-base font-medium text-white">{{ selectedFile?.name || 'Choose file to send' }}</div>
            <div class="mt-1 text-sm text-gray-400">{{ selectedFile ? `${(selectedFile.size / 1024 / 1024).toFixed(2)} MB` : 'Click to open system file picker' }}</div>
          </button>

          <div class="mt-3 grid grid-cols-3 gap-2">
            <button :disabled="!canStartSend" class="rounded-lg bg-sky-500 px-3 py-2 text-sm font-medium text-white shadow-lg transition-all hover:scale-105 disabled:cursor-not-allowed disabled:bg-gray-700 disabled:text-gray-500" @click="startSender">Start</button>
            <button :disabled="!canControlSender" class="rounded-lg bg-gray-800 px-3 py-2 text-sm font-medium text-gray-100 shadow-lg transition-all hover:scale-105 disabled:cursor-not-allowed disabled:text-gray-500" @click="senderState === 'paused' ? resumeSender() : pauseSender()">
              {{ senderState === 'paused' ? 'Resume' : 'Pause' }}
            </button>
            <button :disabled="!canControlSender" class="rounded-lg bg-rose-500/90 px-3 py-2 text-sm font-medium text-white shadow-lg transition-all hover:scale-105 disabled:cursor-not-allowed disabled:bg-gray-700 disabled:text-gray-500" @click="stopSender">End</button>
          </div>

          <div class="mt-3 h-28 overflow-auto rounded-lg border border-white/10 bg-black/30 p-2 font-mono text-xs leading-5 text-gray-300">
            <div v-for="(line, idx) in senderLogs" :key="idx" class="whitespace-nowrap">{{ line }}</div>
          </div>
        </div>

        <div class="rounded-xl border border-white/10 bg-gray-900/75 p-4 shadow-glow backdrop-blur-xl">
          <div class="mb-3 flex items-center justify-between">
            <div>
              <h2 class="text-lg font-semibold text-white">Receiver</h2>
              <p class="text-xs text-gray-400">Directory output uses sender file name</p>
            </div>
            <span class="rounded-lg bg-gray-800 px-2 py-1 text-xs uppercase text-emerald-200">{{ receiverState }}</span>
          </div>

          <div class="grid gap-3 md:grid-cols-[120px_1fr]">
            <div class="grid place-items-center">
              <div class="grid h-28 w-28 place-items-center rounded-full shadow-lg" :style="ringStyle">
                <div class="grid h-20 w-20 place-items-center rounded-full bg-gray-950">
                  <div class="text-center">
                    <div class="text-2xl font-semibold text-white">{{ metrics.progress.toFixed(0) }}%</div>
                    <div class="text-xs text-gray-400">progress</div>
                  </div>
                </div>
              </div>
            </div>
            <div class="grid grid-cols-2 gap-2">
              <div class="rounded-lg bg-gray-800/80 p-3 shadow-lg">
                <div class="text-xs text-gray-400">Speed</div>
                <div class="mt-1 text-xl font-semibold text-white">{{ metrics.speedKBps.toFixed(0) }}</div>
                <div class="text-xs text-gray-500">KB/s</div>
              </div>
              <div class="rounded-lg bg-gray-800/80 p-3 shadow-lg">
                <div class="text-xs text-gray-400">FPS</div>
                <div class="mt-1 text-xl font-semibold text-white">{{ metrics.fps.toFixed(0) }}</div>
                <div class="text-xs text-gray-500">capture/decode</div>
              </div>
              <div class="rounded-lg bg-gray-800/80 p-3 shadow-lg">
                <div class="text-xs text-gray-400">ETA</div>
                <div class="mt-1 text-xl font-semibold text-white">{{ etaText }}</div>
                <div class="text-xs text-gray-500">remaining</div>
              </div>
              <div class="rounded-lg bg-gray-800/80 p-3 shadow-lg">
                <div class="text-xs text-gray-400">Rank</div>
                <div class="mt-1 text-xl font-semibold text-white">{{ metrics.rank }}/{{ metrics.blocks || '--' }}</div>
                <div class="text-xs text-gray-500">fountain</div>
              </div>
            </div>
          </div>

          <div class="mt-3 flex gap-2">
            <input v-model="config.output" class="min-w-0 flex-1 rounded-lg border border-white/10 bg-gray-800 px-3 py-2 text-sm text-gray-100 outline-none transition-all focus:border-sky-400" placeholder="Output directory or file path" />
            <button class="rounded-lg bg-gray-800 px-3 py-2 text-sm text-gray-100 shadow-lg transition-all hover:scale-105 hover:bg-gray-700" @click="chooseOutput">Browse</button>
          </div>

          <div class="mt-3 grid grid-cols-3 gap-2">
            <button class="rounded-lg bg-emerald-500 px-3 py-2 text-sm font-medium text-white shadow-lg transition-all hover:scale-105" @click="startReceiver">Start</button>
            <button :disabled="!canControlReceiver" class="rounded-lg bg-gray-800 px-3 py-2 text-sm font-medium text-gray-100 shadow-lg transition-all hover:scale-105 disabled:cursor-not-allowed disabled:text-gray-500" @click="receiverState === 'paused' ? resumeReceiver() : pauseReceiver()">
              {{ receiverState === 'paused' ? 'Resume' : 'Pause' }}
            </button>
            <button :disabled="!canControlReceiver" class="rounded-lg bg-rose-500/90 px-3 py-2 text-sm font-medium text-white shadow-lg transition-all hover:scale-105 disabled:cursor-not-allowed disabled:bg-gray-700 disabled:text-gray-500" @click="stopReceiver">End</button>
          </div>

          <div class="mt-3 h-24 overflow-auto rounded-lg border border-white/10 bg-black/30 p-2 font-mono text-xs leading-5 text-gray-300">
            <div v-for="(line, idx) in receiverLogs" :key="idx" class="whitespace-nowrap">{{ line }}</div>
          </div>
        </div>
      </section>
    </div>
  </main>
</template>
