import { defineVuetifyConfiguration } from 'vuetify-nuxt-module/custom-configuration'

const lightTheme = {
  dark: false,
  colors: {
    background: '#EEF2EC',
    surface: '#FBFCF8',
    'surface-bright': '#FFFFFF',
    'surface-light': '#F3F6F1',
    'surface-variant': '#D8E1D6',
    'surface-container': '#F4F7F1',
    'surface-container-high': '#EDF2EA',
    'surface-container-highest': '#E7ECE4',
    primary: '#2F6651',
    // 显式覆盖 darken-1:Vuetify 默认主题深合并会残留默认蓝绿色变体
    'primary-darken-1': '#295A47',
    'primary-container': '#BCECD0',
    secondary: '#4B6354',
    'secondary-darken-1': '#42574A',
    'secondary-container': '#CFE9D7',
    'on-secondary-container': '#0F2A1E',
    tertiary: '#7B5B2E',
    'tertiary-container': '#FFDDAF',
    success: '#2F6C4A',
    warning: '#915D00',
    error: '#B3261E',
    'error-container': '#FFDAD6',
    outline: '#6E7B72',
    'outline-variant': '#BBC7BC',
    'on-background': '#171D19',
    'on-surface': '#171D19',
    'on-surface-variant': '#435149',
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
    background: '#0F1511',
    surface: '#141B16',
    'surface-bright': '#313934',
    'surface-light': '#1A221C',
    'surface-variant': '#3D4940',
    'surface-container': '#1A231D',
    'surface-container-high': '#202A23',
    'surface-container-highest': '#273229',
    primary: '#8FD7B4',
    'primary-darken-1': '#7EBD9E',
    'primary-container': '#114E39',
    secondary: '#B0CCBA',
    'secondary-darken-1': '#9BB4A4',
    'secondary-container': '#344A3D',
    'on-secondary-container': '#D8EFE0',
    tertiary: '#F1C48A',
    'tertiary-container': '#5C4216',
    success: '#97D7AF',
    warning: '#F4C06E',
    error: '#FFB4AB',
    'error-container': '#93000A',
    outline: '#85948A',
    'outline-variant': '#3F4B42',
    'on-background': '#DEE5DD',
    'on-surface': '#DEE5DD',
    'on-surface-variant': '#BDC8BD',
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
      rounded: 'pill',
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
      rounded: 'md',
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
