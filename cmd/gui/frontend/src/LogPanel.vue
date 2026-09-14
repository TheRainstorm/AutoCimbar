<script setup lang="ts">
import { nextTick, onMounted, onUnmounted, ref } from 'vue'
import { AppService, type LogEntry } from './runtime/api'
const props = defineProps<{ kind: 'sender' | 'receiver'; detached?: boolean }>()
const entries = ref<LogEntry[]>([])
const paused = ref(false)
const follow = ref(true)
const error = ref('')
const viewport = ref<HTMLElement>()
let cursor = 0
let generation = 0
let busy = false
let clearing = false
let timer: ReturnType<typeof setInterval> | undefined
async function refresh() {
  if (paused.value || busy || clearing) return
  busy = true
  const epoch = generation
  try {
    const batch = await AppService.getLogs(props.kind, cursor)
    if (epoch !== generation || paused.value) return
    cursor = batch.latest
    entries.value = [...entries.value, ...batch.entries].slice(-2000)
    error.value = ''
    if (follow.value && batch.entries.length) {
      await nextTick()
      if (viewport.value) viewport.value.scrollTop = viewport.value.scrollHeight
    }
  } catch (e) { error.value = String(e) }
  finally { busy = false }
}
function togglePause() {
  generation++
  paused.value = !paused.value
  if (!paused.value) void refresh()
}
async function clear() {
  if (clearing) return
  clearing = true
  generation++
  const epoch = generation
  try {
    const batch = await AppService.getLogs(props.kind, cursor)
    if (epoch !== generation) return
    cursor = batch.latest
    entries.value = []
  } catch (e) { error.value = String(e) }
  finally { clearing = false }
}
async function popout() {
  try { await AppService.openLogWindow(props.kind) }
  catch (e) { error.value = String(e) }
}
onMounted(() => { void refresh(); timer = setInterval(refresh, 300) })
onUnmounted(() => { generation++; clearInterval(timer) })
</script>

<template>
  <section class="flex min-h-0 min-w-0 flex-col gap-2 text-gray-200" :class="detached ? 'h-screen bg-gray-950 p-3' : 'mt-2'">
    <div class="flex flex-wrap items-center gap-3 text-xs">
      <strong>{{ kind === 'sender' ? 'Sender' : 'Receiver' }} logs</strong>
      <button class="rounded bg-gray-700 px-2 py-1" @click="clear">Clear</button>
      <button class="rounded bg-gray-700 px-2 py-1" @click="togglePause">{{ paused ? 'Resume logs' : 'Pause logs' }}</button>
      <button v-if="!detached" class="rounded bg-gray-700 px-2 py-1" @click="popout">Pop out</button>
      <label class="flex items-center gap-1"><input v-model="follow" type="checkbox" /> Follow</label>
      <span class="text-gray-400">{{ paused ? 'Display paused; transfer and logging continue' : 'Latest 2000 lines' }}</span>
    </div>
    <div v-if="error" class="text-xs text-rose-300">{{ error }}</div>
    <div ref="viewport" class="min-w-0 overflow-auto rounded-lg border border-white/10 bg-black/30 p-2 font-mono text-xs leading-5 select-text" :class="detached ? 'min-h-0 flex-1' : 'h-24'">
      <div v-for="entry in entries" :key="entry.id" class="w-max min-w-full whitespace-pre">{{ entry.at }} [{{ entry.sessionId }}] {{ entry.message }}</div>
    </div>
  </section>
</template>
