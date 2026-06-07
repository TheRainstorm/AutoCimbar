export type TaskState = 'idle' | 'running' | 'paused' | 'stopped' | 'done' | 'error'

export interface TransferConfig {
  rq: number
  q: number
  screen: number
  cell: string
  ecc: number
  packets: number
  position: string
  scale: number
  fps: number
  output: string
  backend: string
  symbols: string
  noZstd: boolean
  decodeWorkers: number
  captureBackend: string
}

export interface SelectedFile {
  path: string
  name: string
  size: number
}

export interface ScreenInfo {
  index: number
  label: string
  x: number
  y: number
  width: number
  height: number
}

export interface SenderSession {
  id: string
  filePath: string
  fileName: string
  fileSize: number
  state: TaskState
  config: TransferConfig
}

export interface ReceiverSession {
  id: string
  state: TaskState
  config: TransferConfig
}

export interface ReceiverMetrics {
  sessionId: string
  state: TaskState
  progress: number
  speedKBps: number
  fps: number
  etaSeconds: number
  rank: number
  blocks: number
  output: string
  updatedAt: string
}

type BindingMethod = (...args: unknown[]) => Promise<unknown>

interface WailsEventLike {
  data?: unknown
  detail?: unknown
}

const backendPackage = 'github.com/autocambar/autocambar/cmd/gui/internal/backend'
let runtimeScriptPromise: Promise<void> | undefined

function loadRuntimeScript(): Promise<void> {
  if (window.wails?.Call?.ByName) {
    return Promise.resolve()
  }
  if (!runtimeScriptPromise) {
    runtimeScriptPromise = new Promise((resolve) => {
      const existing = document.querySelector<HTMLScriptElement>('script[data-wails-runtime]')
      if (existing) {
        existing.addEventListener('load', () => resolve(), { once: true })
        existing.addEventListener('error', () => resolve(), { once: true })
        return
      }
      const script = document.createElement('script')
      script.type = 'module'
      script.src = '/wails/runtime.js'
      script.dataset.wailsRuntime = 'true'
      script.onload = () => resolve()
      script.onerror = () => resolve()
      document.head.appendChild(script)
    })
  }
  return runtimeScriptPromise
}

function findService(service: string): Record<string, BindingMethod> | undefined {
  const root = window as unknown as Record<string, unknown>
  const bindings = root['go'] ?? root['wails'] ?? root['bindings']
  const candidates = [
    root[service],
    (bindings as Record<string, unknown> | undefined)?.[service],
    (bindings as Record<string, unknown> | undefined)?.[backendPackage] &&
      ((bindings as Record<string, Record<string, unknown>>)[backendPackage] as Record<string, unknown>)[service],
  ]
  return candidates.find((candidate): candidate is Record<string, BindingMethod> => {
    return !!candidate && typeof candidate === 'object'
  })
}

async function call<T>(service: string, method: string, fallback: () => T | Promise<T>, ...args: unknown[]): Promise<T> {
  await loadRuntimeScript()
  if (window.wails?.Call?.ByName) {
    return (await window.wails.Call.ByName(`${backendPackage}.${service}.${method}`, ...args)) as T
  }
  const svc = findService(service)
  const fn = svc?.[method]
  if (typeof fn === 'function') {
    return (await fn(...args)) as T
  }
  return fallback()
}

export const defaultConfig: TransferConfig = {
  rq: 120,
  q: 120,
  screen: 0,
  cell: '8t4s2c',
  ecc: 3,
  packets: 1,
  position: '-0:-0',
  scale: 1,
  fps: 120,
  output: '.',
  backend: 'symbols',
  symbols: '',
  noZstd: false,
  decodeWorkers: 0,
  captureBackend: 'auto',
}

export const AppService = {
  selectFileToSend: () =>
    call<SelectedFile>('AppService', 'SelectFileToSend', () => ({
      path: '',
      name: '',
      size: 0,
    })),
  selectOutputDirectory: () => call<string>('AppService', 'SelectOutputDirectory', () => '.'),
  listScreens: () =>
    call<ScreenInfo[]>('AppService', 'ListScreens', () => [
      { index: 0, label: 'Screen 0', x: 0, y: 0, width: 1920, height: 1080 },
    ]),
  getAutoStart: () => call<boolean>('AppService', 'GetAutoStart', () => false),
  setAutoStart: (enabled: boolean) => call<void>('AppService', 'SetAutoStart', () => undefined, enabled),
  hideMainWindow: () => call<void>('AppService', 'HideMainWindow', () => undefined),
  quit: () => call<void>('AppService', 'Quit', () => undefined),
}

export const ConfigService = {
  getConfig: () => call<TransferConfig>('ConfigService', 'GetConfig', () => defaultConfig),
  saveConfig: (cfg: TransferConfig) => call<TransferConfig>('ConfigService', 'SaveConfig', () => cfg, cfg),
  validateConfig: (cfg: TransferConfig) => call<void>('ConfigService', 'ValidateConfig', () => undefined, cfg),
}

export const EncoderService = {
  prepareSend: (path: string, cfg: TransferConfig) =>
    call<SenderSession>(
      'EncoderService',
      'PrepareSend',
      () => ({
        id: `mock-send-${Date.now()}`,
        filePath: path,
        fileName: path.split(/[\\/]/).pop() ?? 'selected.bin',
        fileSize: 0,
        state: 'idle',
        config: cfg,
      }),
      path,
      cfg,
    ),
  startSend: (id: string) => call<void>('EncoderService', 'StartSend', () => undefined, id),
  pauseSend: (id: string) => call<void>('EncoderService', 'PauseSend', () => undefined, id),
  resumeSend: (id: string) => call<void>('EncoderService', 'ResumeSend', () => undefined, id),
  stopSend: (id: string) => call<void>('EncoderService', 'StopSend', () => undefined, id),
}

export const DecoderService = {
  prepareReceive: (cfg: TransferConfig) =>
    call<ReceiverSession>(
      'DecoderService',
      'PrepareReceive',
      () => ({ id: `mock-recv-${Date.now()}`, state: 'idle', config: cfg }),
      cfg,
    ),
  startReceive: (id: string) => call<void>('DecoderService', 'StartReceive', () => undefined, id),
  pauseReceive: (id: string) => call<void>('DecoderService', 'PauseReceive', () => undefined, id),
  resumeReceive: (id: string) => call<void>('DecoderService', 'ResumeReceive', () => undefined, id),
  stopReceive: (id: string) => call<void>('DecoderService', 'StopReceive', () => undefined, id),
}

export function onEvent<T>(name: string, handler: (payload: T) => void): () => void {
  let stop: (() => void) | undefined
  void loadRuntimeScript().then(() => {
    const off = window.wails?.Events?.On(name, (event: unknown) => handler(unwrapEventPayload<T>(event)))
    if (typeof off === 'function') {
      stop = off
    }
  })
  return () => stop?.()
}

function unwrapEventPayload<T>(event: unknown): T {
  if (event && typeof event === 'object') {
    const ev = event as WailsEventLike
    if ('data' in ev) {
      return ev.data as T
    }
    if ('detail' in ev) {
      return ev.detail as T
    }
  }
  return event as T
}
