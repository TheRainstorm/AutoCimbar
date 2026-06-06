/// <reference types="vite/client" />

declare module '*.vue' {
  import type { DefineComponent } from 'vue'
  const component: DefineComponent<object, object, unknown>
  export default component
}

interface Window {
  wails?: {
    Call?: {
      ByName: (name: string, ...args: unknown[]) => Promise<unknown>
    }
    Events?: {
      On: (name: string, callback: (event: unknown) => void) => (() => void) | void
      Emit: (name: string, ...data: unknown[]) => void
    }
  }
}
