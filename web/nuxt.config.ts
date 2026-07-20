const isDev = process.env.NODE_ENV === 'development'
const baseURL = isDev ? '/' : '/admin/'
const backendURL = process.env.MEOWCLI_BACKEND_URL || 'http://127.0.0.1:3000'

export default defineNuxtConfig({
  compatibilityDate: '2026-03-18',
  devtools: { enabled: false },
  ssr: !isDev,
  srcDir: '.',
  modules: ['vuetify-nuxt-module'],
  css: [
    '~/assets/css/main.css',
  ],
  components: [
    {
      path: '~/components',
      pathPrefix: false,
    },
  ],
  vuetify: {
    moduleOptions: {
      importComposables: true,
      prefixComposables: true,
      styles: true,
    },
    vuetifyOptions: './vuetify.options.ts',
  },
  app: {
    baseURL,
    buildAssetsDir: 'assets/',
    pageTransition: { name: 'page-fade', mode: 'out-in' },
    head: {
      title: 'MeowCLI 管理台',
      htmlAttrs: {
        lang: 'zh-CN',
      },
      meta: [
        { charset: 'utf-8' },
        { name: 'viewport', content: 'width=device-width, initial-scale=1' },
        { name: 'description', content: 'MeowCLI 管理台，用于查看运行状态、管理模型、凭据、日志和访问密钥。' },
        { name: 'color-scheme', content: 'light dark' },
        { name: 'theme-color', content: '#EEF2EC' },
      ],
      link: [
        { rel: 'icon', type: 'image/x-icon', href: `${baseURL}faction.ico` },
      ],
      script: [
        {
          // 首帧前解析主题,避免 SSG 深色用户加载时闪浅色(FOUC)。
          // 与 lib/admin.ts 的 resolveInitialThemePreference/applyTheme 同源:
          // 同一存储键、同一 meta 颜色、同样把 v-theme--* 类写到 <html> 上
          key: 'meowcli-theme-init',
          innerHTML: `(() => {
  try {
    const stored = window.localStorage.getItem('meowcli-admin-theme')
    const prefersDark = window.matchMedia?.('(prefers-color-scheme: dark)').matches
    const theme = stored === 'dark' || (stored !== 'light' && prefersDark) ? 'dark' : 'light'
    const root = document.documentElement
    root.dataset.theme = theme
    root.style.colorScheme = theme
    root.classList.add(theme === 'dark' ? 'v-theme--dark' : 'v-theme--light')
    document.querySelector('meta[name="theme-color"]')?.setAttribute('content', theme === 'dark' ? '#0F1511' : '#EEF2EC')
    document.querySelector('meta[name="color-scheme"]')?.setAttribute('content', theme === 'dark' ? 'dark light' : 'light dark')
  } catch {}
})()`,
        },
      ],
    },
  },
  nitro: {
    routeRules: {
      // 旧版 /dashboard 路径统一重定向到总览首页（生产环境带 /admin/ 前缀）
      '/dashboard': { redirect: isDev ? '/' : `${baseURL}` },
      ...(isDev
        ? {
            '/admin/api/**': { proxy: `${backendURL}/admin/api/**` },
            '/v1/**': { proxy: `${backendURL}/v1/**` },
            '/v1beta/**': { proxy: `${backendURL}/v1beta/**` },
          }
        : {}),
    },
    prerender: {
      routes: ['/', '/dashboard', '/settings', '/credentials', '/models', '/logs', '/keys'],
    },
  },
  typescript: {
    strict: true,
    typeCheck: false,
  },
})
