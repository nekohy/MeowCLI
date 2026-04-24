import { h } from 'vue'
import { aliases as mdiSvgAliases } from 'vuetify/iconsets/mdi-svg'
import type { IconOptions, IconProps } from 'vuetify'
import { mdiIconPaths } from '~/lib/icons'

export default defineNuxtPlugin({
  name: 'meowcli:mdi-subset',
  order: -24,
  parallel: true,
  setup(nuxtApp) {
    nuxtApp.hook('vuetify:configuration', ({ vuetifyOptions }) => {
      vuetifyOptions.icons = {
        defaultSet: 'mdi',
        aliases: mdiSvgAliases,
        sets: {
          mdi: {
            component: (props: IconProps) => {
              const icon = typeof props.icon === 'string' ? props.icon : ''
              const path = icon.startsWith('svg:') ? icon.slice(4) : mdiIconPaths[icon]
              const { icon: _icon, tag, ...attrs } = props as IconProps & Record<string, unknown>
              void _icon

              return h(tag, { ...attrs, class: ['v-icon--svg', attrs.class] }, [
                h('svg', {
                  class: 'v-icon__svg',
                  xmlns: 'http://www.w3.org/2000/svg',
                  viewBox: '0 0 24 24',
                  role: 'img',
                  'aria-hidden': 'true',
                }, [
                  h('path', { d: path || mdiIconPaths['mdi-square-rounded-outline'] }),
                ]),
              ])
            },
          },
        },
      } satisfies IconOptions
    })
  },
})
