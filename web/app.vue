<script setup lang="ts">
import { AUTH_INVALID_EVENT } from '~/composables/useAdminApi'
import {
  NAV_ITEMS,
  THEME_STORAGE_KEY,
  applyTheme,
  colorForTone,
  copyText,
} from '~/lib/admin'

const admin = useAdminApp()
const route = useRoute()
const router = useRouter()
const runtimeConfig = useRuntimeConfig()
const display = useVDisplay()
const vuetifyTheme = useVTheme()
const defaultNav = NAV_ITEMS[0]!

const clientReady = ref(false)
const sessionReady = ref(false)
const drawer = ref(false)

const faviconPath = computed(() => `${runtimeConfig.app.baseURL}faction.ico`)

const currentNav = computed(() => {
  const routeKey = String(route.meta.navKey || 'dashboard')
  return NAV_ITEMS.find((item) => item.key === routeKey) || defaultNav
})

const onlineHandlerCount = computed(() => (
  admin.handlers.value.filter((item) => item.status === 'enabled' || item.status === 'available').length
))

const authCardMeta = computed(() => {
  if (admin.setupDone.value && admin.setupResult.value) {
    return {
      eyebrow: '引导',
      title: '保存管理员密钥',
      description: '该密钥只展示一次。',
      chip: '已创建',
      color: 'success',
    }
  }

  if (admin.needSetup.value) {
    return {
      eyebrow: '初始化',
      title: '初始化管理员',
      description: '创建首个管理员密钥。',
      chip: '首次启动',
      color: 'primary',
    }
  }

  return {
    eyebrow: '安全访问',
    title: '管理员登录',
    description: '输入管理员密钥后进入控制台。',
    chip: '需要验证',
    color: 'secondary',
  }
})

const pageTitle = computed(() => {
  if (!admin.authReady.value) {
    return 'MeowCLI 管理台'
  }

  return `${currentNav.value.label} | MeowCLI 管理台`
})

const snackbarOpen = computed({
  get: () => Boolean(admin.toast.value),
  set: (value: boolean) => {
    if (!value) {
      admin.dismissToast()
    }
  },
})

const snackbarColor = computed(() => colorForTone(admin.toast.value?.tone))

useHead(() => ({
  title: pageTitle.value,
}))

function persistTheme(theme: string) {
  if (!import.meta.client) {
    return
  }

  vuetifyTheme.change(theme as 'light' | 'dark')
  applyTheme(theme as 'light' | 'dark')
  window.localStorage.setItem(THEME_STORAGE_KEY, theme)
}

async function handleLogin() {
  if (await admin.submitLogin()) {
    await router.push('/')
  }
}

function handleAuthInvalid() {
  admin.resetAuthState('管理员密钥无效或已失效')
}

async function copySetupKey() {
  const value = admin.setupResult.value?.key
  if (!value) {
    return
  }

  if (await copyText(value)) {
    admin.notify('管理员密钥已复制')
  } else {
    admin.notify('复制失败，请手动复制', 'warning')
  }
}

let toastTimer: number | undefined

watch(
  () => admin.theme.value,
  (theme) => {
    if (!import.meta.client || !clientReady.value) {
      return
    }

    persistTheme(theme)
  },
)

watch(
  () => admin.toast.value,
  (toast) => {
    if (!import.meta.client) {
      return
    }

    if (toastTimer) {
      window.clearTimeout(toastTimer)
    }

    if (toast) {
      toastTimer = window.setTimeout(() => admin.dismissToast(), 2400)
    }
  },
)

watch(
  () => display.mdAndUp.value,
  (desktop) => {
    drawer.value = desktop
  },
  { immediate: true },
)

watch(
  () => route.fullPath,
  () => {
    if (!display.mdAndUp.value) {
      drawer.value = false
    }
  },
)

onMounted(() => {
  admin.initializeClient()
  clientReady.value = true
  persistTheme(admin.theme.value)
  window.addEventListener(AUTH_INVALID_EVENT, handleAuthInvalid)

  void (async () => {
    try {
      await admin.boot()
    } finally {
      sessionReady.value = true
    }
  })()
})

onBeforeUnmount(() => {
  if (!import.meta.client) {
    return
  }

  window.removeEventListener(AUTH_INVALID_EVENT, handleAuthInvalid)
  if (toastTimer) {
    window.clearTimeout(toastTimer)
  }
})
</script>

<template>
  <VApp class="admin-app">
    <VSnackbar
      v-model="snackbarOpen"
      :color="snackbarColor"
      location="top end"
      timeout="2400"
      class="app-snackbar"
    >
      {{ admin.toast.value?.text }}
      <template #actions>
        <VBtn variant="text" @click="admin.dismissToast()">关闭</VBtn>
      </template>
    </VSnackbar>

    <!-- Loading state -->
    <template v-if="!clientReady || !sessionReady">
      <VMain class="shell-stage shell-stage--center">
        <VContainer class="shell-container d-flex align-center fill-height">
          <VRow justify="center" class="w-100">
            <VCol cols="12" sm="10" md="8" lg="6">
              <VCard color="surface-container" variant="flat">
                <VCardText class="pa-6 pa-md-8 d-grid ga-6">
                  <div class="d-flex align-center ga-4">
                    <VAvatar size="56" color="primary-container" rounded="xl">
                      <img :src="faviconPath" alt="" class="brand-image">
                    </VAvatar>
                      <div class="loading-copy">
                        <div class="text-overline loading-eyebrow">管理台</div>
                        <div class="text-h5 font-weight-bold">恢复控制台会话</div>
                        <div class="text-body-2 text-medium-emphasis">正在恢复本地状态。</div>
                      </div>
                    </div>
                  <VProgressLinear indeterminate rounded color="primary" />
                </VCardText>
              </VCard>
            </VCol>
          </VRow>
        </VContainer>
      </VMain>
    </template>

    <!-- Auth state — centered login card -->
    <template v-else-if="!admin.authReady.value">
      <VMain class="shell-stage shell-stage--center">
        <VContainer class="shell-container">
          <VRow justify="center">
            <VCol cols="12" sm="10" md="8" lg="6">
              <div class="d-flex flex-column align-center ga-6 mb-8">
                <VAvatar size="72" color="primary-container" rounded="xl">
                  <img :src="faviconPath" alt="" class="brand-image">
                </VAvatar>
                <div class="text-center">
                  <div class="text-overline" style="color: rgb(var(--v-theme-primary))">MEOWCLI</div>
                  <h1 class="text-h4 font-weight-bold">管理控制台</h1>
                </div>
              </div>

              <VCard color="surface-container" variant="flat">
                <VCardItem class="pa-6 pb-0">
                  <template #prepend>
                    <VAvatar size="48" color="primary-container" rounded="xl">
                      <VIcon icon="mdi-shield-lock-outline" color="primary" size="22" />
                    </VAvatar>
                  </template>
                  <VCardSubtitle class="text-wrap">{{ authCardMeta.eyebrow }}</VCardSubtitle>
                  <VCardTitle class="text-h5 font-weight-bold">{{ authCardMeta.title }}</VCardTitle>
                  <template #append>
                    <VChip :color="authCardMeta.color" variant="tonal" size="small">
                      {{ authCardMeta.chip }}
                    </VChip>
                  </template>
                </VCardItem>

                <VCardText class="auth-card-body">
                  <p class="text-body-2 text-medium-emphasis" style="line-height: 1.65">
                    {{ authCardMeta.description }}
                  </p>

                  <template v-if="admin.setupDone.value && admin.setupResult.value">
                    <div class="d-grid ga-4">
                      <VSheet rounded="xl" color="surface-container-high" class="pa-4">
                        <code class="key-code">{{ admin.setupResult.value.key }}</code>
                      </VSheet>

                      <div class="d-flex flex-wrap ga-2">
                        <AdminButton
                          variant="secondary"
                          prepend-icon="mdi-content-copy"
                          @click="copySetupKey"
                        >
                          复制密钥
                        </AdminButton>
                        <AdminButton
                          prepend-icon="mdi-login"
                          :loading="admin.booting.value"
                          @click="handleLogin"
                        >
                          立即登录
                        </AdminButton>
                      </div>

                      <VAlert
                        v-if="admin.loginError.value"
                        type="error"
                        variant="tonal"
                        density="comfortable"
                        :text="admin.loginError.value"
                      />
                    </div>
                  </template>

                  <template v-else-if="admin.needSetup.value">
                    <form class="auth-form" @submit.prevent="admin.setupAdmin()">
                      <VTextField
                        v-model="admin.setupState.value.key"
                        label="自定义密钥"
                        placeholder="留空自动生成"
                        prepend-inner-icon="mdi-key-outline"
                      />
                      <VTextField
                        v-model="admin.setupState.value.note"
                        label="备注"
                        placeholder="例如：本地管理员"
                        prepend-inner-icon="mdi-note-outline"
                      />
                      <VAlert
                        v-if="admin.loginError.value"
                        type="error"
                        variant="tonal"
                        density="comfortable"
                        :text="admin.loginError.value"
                      />
                      <AdminButton
                        type="submit"
                        block
                        prepend-icon="mdi-shield-plus-outline"
                        :loading="admin.booting.value"
                      >
                        创建并进入
                      </AdminButton>
                    </form>
                  </template>

                  <template v-else>
                    <form class="auth-form" @submit.prevent="handleLogin">
                      <VTextField
                        v-model="admin.loginInput.value"
                        label="访问密钥"
                        type="password"
                        autocomplete="current-password"
                        placeholder="sk-..."
                        prepend-inner-icon="mdi-lock-outline"
                      />
                      <VAlert
                        v-if="admin.loginError.value"
                        type="error"
                        variant="tonal"
                        density="comfortable"
                        :text="admin.loginError.value"
                      />
                      <AdminButton
                        type="submit"
                        block
                        prepend-icon="mdi-login"
                        :loading="admin.booting.value"
                      >
                        登录控制台
                      </AdminButton>
                    </form>
                  </template>
                </VCardText>
              </VCard>
            </VCol>
          </VRow>
        </VContainer>
      </VMain>
    </template>

    <!-- Authenticated shell -->
    <template v-else>
      <VLayout class="admin-layout">
        <VNavigationDrawer
          v-model="drawer"
          :permanent="display.mdAndUp.value"
          :temporary="display.smAndDown.value"
          width="216"
          class="admin-drawer"
        >
          <div class="drawer-layout">
            <VSheet class="drawer-brand-surface" color="surface-container-high" rounded="xl">
              <div class="drawer-brand-row">
                <VAvatar size="40" color="primary-container" rounded="xl">
                  <img :src="faviconPath" alt="" class="brand-image">
                </VAvatar>
                <div class="drawer-brand-copy">
                  <div class="text-overline drawer-brand-eyebrow">MEOWCLI</div>
                  <div class="drawer-brand-title">管理台</div>
                </div>
              </div>
            </VSheet>

            <VList nav density="compact" class="nav-list">
              <VListItem
                v-for="item in NAV_ITEMS"
                :key="item.key"
                :to="item.to"
                color="primary"
                class="nav-item"
                :active="currentNav.key === item.key"
              >
                <template #prepend>
                  <VIcon :icon="item.icon" :color="currentNav.key === item.key ? 'primary' : 'on-surface-variant'" size="20" />
                </template>
                <VListItemTitle class="nav-item-title">{{ item.label }}</VListItemTitle>
              </VListItem>
            </VList>

          </div>
        </VNavigationDrawer>

        <VAppBar height="68" class="app-bar" rounded="xl" style="margin: 12px 12px 0">
          <VAppBarNavIcon v-if="display.smAndDown.value" @click="drawer = !drawer" />

          <div class="app-bar-copy">
            <div class="app-bar-title">{{ currentNav.label }}</div>
          </div>

          <template #append>
            <ThemeToggle
              :theme="admin.theme.value"
              @toggle="admin.toggleTheme()"
            />
            <VBtn
              variant="text"
              color="primary"
              icon="mdi-logout"
              @click="admin.logout()"
            />
          </template>

          <VProgressLinear
            :active="admin.booting.value"
            :model-value="admin.booting.value ? 100 : 0"
            indeterminate
            color="primary"
            absolute
            location="bottom"
          />
        </VAppBar>

        <VMain class="admin-main">
          <VContainer class="app-container">
            <NuxtPage />
          </VContainer>
        </VMain>
      </VLayout>
    </template>
  </VApp>
</template>
