// Wails 绑定包装：window.go.main.App 由 wails dev / build 注入
export const App = new Proxy({}, {
  get(_target, name) {
    if (typeof name !== 'string') return undefined
    return (...args) => {
      const api = window.go?.main?.App
      if (!api || typeof api[name] !== 'function') {
        return Promise.reject(new Error(`Wails 绑定未就绪（${name}），请通过 wails dev 或打包后的程序运行`))
      }
      return Promise.resolve(api[name](...args))
    }
  },
})

export const rt = () => window.runtime

export const onEvent = (name, handler) => {
  try {
    window.runtime?.EventsOn?.(name, handler)
  } catch (err) {
    console.warn('事件注册失败', name, err)
  }
}
