const isDev = process.env.NODE_ENV === 'development'
const baseURL = isDev ? '/' : '/admin/'
const backendURL = process.env.MEOWCLI_BACKEND_URL || 'http://127.0.0.1:3000'

export default defineNuxtConfig({
  compatibilityDate: '2026-03-18',
  devtools: { enabled: false },
  ssr: true,
  srcDir: '.',
  modules: ['vuetify-nuxt-module'],
  css: [
    '@mdi/font/css/materialdesignicons.css',
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
        { name: 'color-scheme', content: 'light dark' },
        { name: 'theme-color', content: '#EEF2EC' },
      ],
      link: [
        { rel: 'icon', type: 'image/x-icon', href: `${baseURL}faction.ico` },
        { rel: 'preconnect', href: 'https://fonts.googleapis.com' },
        { rel: 'preconnect', href: 'https://fonts.gstatic.com', crossorigin: '' },
        { rel: 'stylesheet', href: 'https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700;800&family=Noto+Sans+SC:wght@400;500;600;700;800&display=swap' },
      ],
    },
  },
  nitro: {
    routeRules: isDev
      ? {
          '/admin/api/**': { proxy: `${backendURL}/admin/api/**` },
          '/v1/**': { proxy: `${backendURL}/v1/**` },
          '/v1beta/**': { proxy: `${backendURL}/v1beta/**` },
        }
      : {},
    prerender: {
      routes: ['/', '/dashboard', '/settings', '/credentials', '/models', '/logs', '/keys'],
    },
  },
  typescript: {
    strict: true,
    typeCheck: false,
  },
})
