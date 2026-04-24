import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, resolve } from 'node:path'
import assert from 'node:assert/strict'

const scriptDir = dirname(fileURLToPath(import.meta.url))
const appVue = readFileSync(resolve(scriptDir, '../app.vue'), 'utf8')

const needSetupBlockMatch = appVue.match(/if \(admin\.needSetup\.value\) \{\s*return \{(?<body>[\s\S]*?)\n\s*\}\n\s*\}/)
assert.ok(needSetupBlockMatch?.groups?.body, 'Could not find needSetup auth card metadata block')

const needSetupBlock = needSetupBlockMatch.groups.body

assert.match(needSetupBlock, /title:\s*'初始化'/, 'Initial setup card title should be exactly 初始化')
assert.doesNotMatch(needSetupBlock, /title:\s*'初始化管理员'/, 'Initial setup card should not render 初始化管理员')
assert.match(needSetupBlock, /eyebrow:\s*''/, 'Initial setup card eyebrow should be empty to avoid duplicate 初始化 copy')
assert.match(appVue, /<VCardSubtitle\s+v-if="authCardMeta\.eyebrow"/, 'Auth card subtitle should render only when eyebrow text exists')
