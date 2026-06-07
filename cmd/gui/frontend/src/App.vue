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

const isLite = import.meta.env.VITE_AUTOCIMBAR_LITE === '1'
const config = reactive<TransferConfig>({ ...defaultConfig })
const screens = ref<ScreenInfo[]>([])
const selectedFile = ref<SelectedFile | null>(null)
const sender = ref<SenderSession | null>(null)
const receiver = ref<ReceiverSession | null>(null)
const senderState = ref<TaskState>('idle')
const receiverState = ref<TaskState>('idle')
const advancedOpen = ref(false)
const selectedPlacement = computed({
  get: () => placementFromPosition(config.position),
  set: (value: string) => {
    config.position = positionFromPlacement(value)
  },
})
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

const tips = {
  rq: 'Reference grid size using 8x8 tiles; actual Q scales when tile size changes.',
  screen: 'Display index used by the sender window and receiver capture region.',
  captureBackend:
    'Receiver screen capture backend. DXGI is fastest, but HDR/color-managed displays can break high color-bit modes; use SDR or GDI when colors do not decode.',
  backend: 'Frame backend. symbols is the high-throughput AutoCimBar path; qr is for QR-code comparison.',
  cell: 'Frame format: compact cell spec with tile size, shape bits, and color bits. Sender and receiver must match.',
  ecc: 'Frame format: per-packet Reed-Solomon ECC percentage. Sender and receiver must match.',
  packets: 'Frame format: independent packets packed into each screen frame. Sender and receiver must match.',
  zstd: 'Frame format: zstd source compression is enabled by default. Sender controls it; receiver detects it from transfer metadata.',
  scale: 'Screen scale factor B. Increase when the display path needs larger pixels.',
  fps: 'Target sender refresh or receiver capture frame rate.',
  placement: 'Window/capture placement on the selected screen. CLI still supports exact X:Y.',
  output: 'Output directory or file path. Directories use the sender file name.',
}

function pushLog(target: typeof senderLogs | typeof receiverLogs, message: string) {
  target.value = [...target.value.slice(-80), message]
}

function placementFromPosition(position: string): string {
  switch ((position || '').trim()) {
    case '0:0':
      return 'top-left'
    case '-0:0':
      return 'top-right'
    case '0:-0':
      return 'bottom-left'
    case 'c:c':
      return 'center'
    case '-0:-0':
    default:
      return 'bottom-right'
  }
}

function positionFromPlacement(placement: string): string {
  switch (placement) {
    case 'top-left':
      return '0:0'
    case 'top-right':
      return '-0:0'
    case 'bottom-left':
      return '0:-0'
    case 'center':
      return 'c:c'
    case 'bottom-right':
    default:
      return '-0:-0'
  }
}

async function loadInitial() {
  Object.assign(config, await ConfigService.getConfig())
  applyLiteConfig()
  screens.value = await AppService.listScreens()
}

function applyLiteConfig() {
  if (!isLite) return
  if (!config.rq || config.rq < 1) config.rq = 26
  if (config.rq > 40) config.rq = 40
  if (!config.scale || config.scale < 1) config.scale = 1
  config.cell = '8t4s2c'
  config.ecc = 3
  config.packets = 1
  config.fps = 30
  config.backend = 'symbols'
  config.captureBackend = 'gdi'
  config.noZstd = false
  config.symbols = ''
  config.decodeWorkers = 0
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

async function minimizeToTray() {
  await AppService.hideMainWindow()
}

async function startSender() {
  if (!selectedFile.value) return
  applyLiteConfig()
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
  applyLiteConfig()
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
    <div class="flex min-h-full w-full min-w-0 flex-col gap-3 px-4 py-3">
      <header class="flex items-center justify-between">
        <div>
          <h1 class="text-lg font-semibold text-white">{{ isLite ? 'AutoCimBar Lite' : 'AutoCimBar' }}</h1>
          <p class="text-xs text-gray-400">{{ isLite ? 'Simple sender for small screen transfers' : 'High-speed one-way screen channel file transfer' }}</p>
        </div>
        <button class="rounded-lg border border-white/10 bg-gray-800 px-3 py-2 text-xs text-gray-100 shadow-lg transition-all hover:scale-105 hover:bg-gray-700" title="Hide the main window to the system tray." @click="minimizeToTray">
          To Tray
        </button>
      </header>

      <section class="min-w-0 rounded-xl border border-white/10 bg-gray-900/80 p-3 shadow-lg backdrop-blur-xl">
        <div class="grid min-w-0 gap-3" :class="isLite ? 'md:grid-cols-[100px_1fr_90px_170px]' : 'md:grid-cols-[100px_1fr_170px_auto]'">
          <label class="block" :title="tips.rq">
            <span class="text-xs text-gray-400">RQ</span>
            <input v-model.number="config.rq" type="number" min="1" :max="isLite ? 40 : undefined" class="mt-1 h-9 w-full rounded-lg border border-white/10 bg-gray-800 px-3 text-sm text-gray-100 outline-none focus:border-sky-400" @change="applyLiteConfig" />
          </label>
          <label class="block" :title="tips.screen">
            <span class="text-xs text-gray-400">Screen</span>
            <select v-model.number="config.screen" class="mt-1 h-9 w-full rounded-lg border border-white/10 bg-gray-800 px-3 text-sm text-gray-100 outline-none focus:border-sky-400">
              <option v-for="screen in screens" :key="screen.index" :value="screen.index">{{ screen.label }}</option>
            </select>
          </label>
          <label v-if="!isLite" class="block" :title="tips.captureBackend">
            <span class="text-xs text-gray-400">Capture</span>
            <select v-model="config.captureBackend" class="mt-1 h-9 w-full rounded-lg border border-white/10 bg-gray-800 px-3 text-sm text-gray-100 outline-none focus:border-sky-400">
              <option value="auto">auto</option>
              <option value="dxgi">dxgi</option>
              <option value="gdi">gdi</option>
            </select>
          </label>
          <label v-if="isLite" class="block" :title="tips.scale">
            <span class="text-xs text-gray-400">B</span>
            <input v-model.number="config.scale" type="number" min="1" class="mt-1 h-9 w-full rounded-lg border border-white/10 bg-gray-800 px-3 text-sm text-gray-100 outline-none focus:border-sky-400" @change="applyLiteConfig" />
          </label>
          <label v-if="isLite" class="block" :title="tips.placement">
            <span class="text-xs text-gray-400">Placement</span>
            <select v-model="selectedPlacement" class="mt-1 h-9 w-full rounded-lg border border-white/10 bg-gray-800 px-3 text-sm text-gray-100 outline-none focus:border-sky-400">
              <option value="bottom-right">Bottom right</option>
              <option value="bottom-left">Bottom left</option>
              <option value="top-right">Top right</option>
              <option value="top-left">Top left</option>
              <option value="center">Center</option>
            </select>
          </label>
          <button v-if="!isLite" class="self-end rounded-lg border border-white/10 bg-gray-800 px-4 py-2 text-sm text-gray-100 shadow-lg transition-all hover:scale-105 hover:bg-gray-700" @click="advancedOpen = !advancedOpen">
            Advanced
          </button>
        </div>

        <div v-if="!isLite" class="grid overflow-hidden transition-all duration-300 ease-out" :class="advancedOpen ? 'grid-rows-[1fr] opacity-100' : 'grid-rows-[0fr] opacity-0'">
          <div class="min-h-0">
            <div class="mt-3 rounded-xl border border-cyan-300/20 bg-cyan-500/10 p-3 shadow-lg">
              <div class="mb-2 text-xs font-semibold uppercase tracking-wide text-cyan-100">Frame format - both sides must match</div>
              <div class="grid gap-3 md:grid-cols-5">
                <label class="block" :title="tips.backend">
                  <span class="text-xs text-cyan-100">Backend</span>
                  <select v-model="config.backend" class="mt-1 h-9 w-full rounded-lg border border-cyan-200/20 bg-gray-950/70 px-3 text-sm text-gray-100 outline-none focus:border-cyan-300">
                    <option value="symbols">symbols</option>
                    <option value="qr">qr</option>
                  </select>
                </label>
                <label class="block" :title="tips.cell">
                  <span class="text-xs text-cyan-100">Cell (-c)</span>
                  <input v-model="config.cell" class="mt-1 h-9 w-full rounded-lg border border-cyan-200/20 bg-gray-950/70 px-3 text-sm text-gray-100 outline-none focus:border-cyan-300" />
                </label>
                <label class="block" :title="tips.ecc">
                  <span class="text-xs text-cyan-100">ECC</span>
                  <input v-model.number="config.ecc" type="number" min="0" max="100" class="mt-1 h-9 w-full rounded-lg border border-cyan-200/20 bg-gray-950/70 px-3 text-sm text-gray-100 outline-none focus:border-cyan-300" />
                </label>
                <label class="block" :title="tips.packets">
                  <span class="text-xs text-cyan-100">Packets (-p)</span>
                  <input v-model.number="config.packets" type="number" min="1" class="mt-1 h-9 w-full rounded-lg border border-cyan-200/20 bg-gray-950/70 px-3 text-sm text-gray-100 outline-none focus:border-cyan-300" />
                </label>
                <label class="flex items-center gap-2 self-end rounded-lg border border-cyan-200/20 bg-gray-950/70 px-3 py-2 text-sm text-gray-100" :title="tips.zstd">
                  <input v-model="config.noZstd" type="checkbox" class="h-4 w-4 accent-cyan-400" />
                  <span>No zstd</span>
                </label>
              </div>
            </div>

            <div class="mt-3 grid gap-3 md:grid-cols-3">
              <label class="block" :title="tips.scale">
                <span class="text-xs text-gray-400">B</span>
                <input v-model.number="config.scale" type="number" min="1" class="mt-1 h-9 w-full rounded-lg border border-white/10 bg-gray-800 px-3 text-sm text-gray-100 outline-none focus:border-sky-400" />
              </label>
              <label class="block" :title="tips.fps">
                <span class="text-xs text-gray-400">FPS</span>
                <input v-model.number="config.fps" type="number" min="1" class="mt-1 h-9 w-full rounded-lg border border-white/10 bg-gray-800 px-3 text-sm text-gray-100 outline-none focus:border-sky-400" />
              </label>
              <label class="block" :title="tips.placement">
                <span class="text-xs text-gray-400">Placement</span>
                <select v-model="selectedPlacement" class="mt-1 h-9 w-full rounded-lg border border-white/10 bg-gray-800 px-3 text-sm text-gray-100 outline-none focus:border-sky-400">
                  <option value="bottom-right">Bottom right</option>
                  <option value="bottom-left">Bottom left</option>
                  <option value="top-right">Top right</option>
                  <option value="top-left">Top left</option>
                  <option value="center">Center</option>
                </select>
              </label>
            </div>
          </div>
        </div>
      </section>

      <section class="grid min-w-0 grid-cols-1 gap-3" :class="isLite ? 'lg:grid-cols-1' : 'lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]'">
        <div v-if="!isLite" class="min-w-0 rounded-xl border border-white/10 bg-gray-900/75 p-4 shadow-glow backdrop-blur-xl">
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

          <div class="mt-3 h-28 min-w-0 max-w-full overflow-x-auto overflow-y-auto rounded-lg border border-white/10 bg-black/30 p-2 font-mono text-xs leading-5 text-gray-300">
            <div v-for="(line, idx) in senderLogs" :key="idx" class="w-max min-w-full whitespace-nowrap">{{ line }}</div>
          </div>
        </div>

        <div class="min-w-0 rounded-xl border border-white/10 bg-gray-900/75 p-4 shadow-glow backdrop-blur-xl">
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
            <input v-model="config.output" class="min-w-0 flex-1 rounded-lg border border-white/10 bg-gray-800 px-3 py-2 text-sm text-gray-100 outline-none transition-all focus:border-sky-400" placeholder="Output directory or file path" :title="tips.output" />
            <button class="rounded-lg bg-gray-800 px-3 py-2 text-sm text-gray-100 shadow-lg transition-all hover:scale-105 hover:bg-gray-700" :title="tips.output" @click="chooseOutput">Browse</button>
          </div>

          <div class="mt-3 grid grid-cols-3 gap-2">
            <button class="rounded-lg bg-emerald-500 px-3 py-2 text-sm font-medium text-white shadow-lg transition-all hover:scale-105" @click="startReceiver">Start</button>
            <button :disabled="!canControlReceiver" class="rounded-lg bg-gray-800 px-3 py-2 text-sm font-medium text-gray-100 shadow-lg transition-all hover:scale-105 disabled:cursor-not-allowed disabled:text-gray-500" @click="receiverState === 'paused' ? resumeReceiver() : pauseReceiver()">
              {{ receiverState === 'paused' ? 'Resume' : 'Pause' }}
            </button>
            <button :disabled="!canControlReceiver" class="rounded-lg bg-rose-500/90 px-3 py-2 text-sm font-medium text-white shadow-lg transition-all hover:scale-105 disabled:cursor-not-allowed disabled:bg-gray-700 disabled:text-gray-500" @click="stopReceiver">End</button>
          </div>

          <div class="mt-3 h-24 min-w-0 max-w-full overflow-x-auto overflow-y-auto rounded-lg border border-white/10 bg-black/30 p-2 font-mono text-xs leading-5 text-gray-300">
            <div v-for="(line, idx) in receiverLogs" :key="idx" class="w-max min-w-full whitespace-nowrap">{{ line }}</div>
          </div>
        </div>
      </section>
    </div>
  </main>
</template>
