import { defineVuetifyConfiguration } from 'vuetify-nuxt-module/custom-configuration'

const lightTheme = {
  dark: false,
  colors: {
    background: '#ECF0E9',
    surface: '#FCFDFA',
    'surface-bright': '#FFFFFF',
    'surface-light': '#F3F6F1',
    'surface-variant': '#D8E1D6',
    'surface-container': '#F2F5F0',
    'surface-container-high': '#E9EEE7',
    'surface-container-highest': '#E0E6DE',
    primary: '#1D6D4D',
    // 显式覆盖 darken-1:Vuetify 默认主题深合并会残留默认蓝绿色变体
    'primary-darken-1': '#196043',
    'primary-container': '#A9F0C2',
    secondary: '#3E6B58',
    'secondary-darken-1': '#375E4D',
    'secondary-container': '#C7EDD8',
    'on-secondary-container': '#0F2A1E',
    tertiary: '#8C5E1B',
    'tertiary-container': '#FFDDAF',
    success: '#1E7A4B',
    warning: '#915D00',
    error: '#B3261E',
    'error-container': '#FFDAD6',
    outline: '#6E7B72',
    'outline-variant': '#BBC7BC',
    // 阴影基色：main.css 阴影令牌走 rgb(var(--v-theme-shadow))，必须显式入色板才会生成变量
    shadow: '#000000',
    'on-background': '#141B16',
    'on-surface': '#141B16',
    'on-surface-variant': '#404D44',
    'on-primary': '#FFFFFF',
    'on-primary-darken-1': '#FFFFFF',
    'on-primary-container': '#032115',
    'on-secondary': '#FFFFFF',
    'on-secondary-darken-1': '#FFFFFF',
    'on-tertiary': '#FFFFFF',
    'on-error': '#FFFFFF',
  },
}

const darkTheme = {
  dark: true,
  colors: {
    background: '#121916',
    surface: '#171F1B',
    'surface-bright': '#3A433D',
    'surface-light': '#1E2722',
    'surface-variant': '#465349',
    'surface-container': '#1F2823',
    'surface-container-high': '#29332D',
    'surface-container-highest': '#343E37',
    primary: '#96E0B8',
    'primary-darken-1': '#85C9A4',
    'primary-container': '#146044',
    secondary: '#A8D6BF',
    'secondary-darken-1': '#92BFA7',
    'secondary-container': '#2E5344',
    'on-secondary-container': '#D8EFE0',
    tertiary: '#F1C48A',
    'tertiary-container': '#5C4216',
    success: '#97D7AF',
    warning: '#F4C06E',
    error: '#FFB4AB',
    'error-container': '#93000A',
    outline: '#97A69B',
    'outline-variant': '#4A574D',
    shadow: '#000000',
    'on-background': '#E2E9E1',
    'on-surface': '#E2E9E1',
    'on-surface-variant': '#CBD5CA',
    'on-primary': '#003826',
    'on-primary-darken-1': '#003220',
    'on-primary-container': '#BCECD0',
    'on-secondary': '#1C3428',
    'on-secondary-darken-1': '#182D23',
    'on-tertiary': '#462E06',
    'on-error': '#690005',
  },
}

export default defineVuetifyConfiguration({
  defaults: {
    VAppBar: {
      flat: true,
      color: 'surface-container',
    },
    VBtn: {
      // 圆角由 CSS 令牌 --admin-radius-control* 统一接管，消除双源
      variant: 'tonal',
      color: 'primary',
      height: 40,
      elevation: 0,
    },
    VCard: {
      rounded: 'lg',
      elevation: 0,
      border: false,
      VBtn: {
        variant: 'text',
        slim: true,
      },
    },
    VChip: {
      rounded: 'lg',
      size: 'default',
    },
    VDialog: {
      maxWidth: 520,
    },
    VExpansionPanel: {
      elevation: 0,
      rounded: 'lg',
    },
    VList: {
      bgColor: 'transparent',
    },
    VListItem: {
      rounded: 'lg',
      minHeight: 44,
    },
    VNavigationDrawer: {
      elevation: 0,
      color: 'surface-container',
    },
    VPagination: {
      activeColor: 'primary',
      rounded: 'lg',
    },
    VSelect: {
      color: 'primary',
      variant: 'outlined',
      rounded: 'md',
      density: 'comfortable',
      hideDetails: 'auto',
      menuProps: {
        contentClass: 'admin-select-menu',
        offset: 6,
      },
    },
    VSnackbar: {
      rounded: 'lg',
      elevation: 0,
    },
    VSwitch: {
      color: 'primary',
      hideDetails: true,
      inset: true,
      density: 'compact',
    },
    VTextField: {
      color: 'primary',
      variant: 'outlined',
      rounded: 'md',
      density: 'comfortable',
      hideDetails: 'auto',
    },
    VTextarea: {
      color: 'primary',
      variant: 'outlined',
      rounded: 'md',
      density: 'comfortable',
      hideDetails: 'auto',
      autoGrow: true,
    },
    VToolbar: {
      VBtn: {
        variant: 'text',
      },
    },
  },
  display: {
    mobileBreakpoint: 'md',
  },
  icons: {
    defaultSet: 'custom',
  },
  theme: {
    defaultTheme: 'light',
    // 关闭颜色变体生成：默认会为 primary/secondary 等生成 lighten/darken 变体，
    // 深合并后产物里残留 Vuetify 默认蓝色变体，与自定义绿灰主色冲突
    variations: {
      colors: [],
      lighten: 0,
      darken: 0,
    },
    themes: {
      light: lightTheme,
      dark: darkTheme,
    },
  },
})
