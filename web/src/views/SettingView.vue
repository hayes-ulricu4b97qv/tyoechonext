<script setup lang="ts">
import { reactive, ref } from 'vue'
import { ApiError, postJSON, putJSON } from '../api'
import type { Site, Status } from '../types'

const props = defineProps<{ status: Status }>()
const initialized = props.status.initialized
const submitting = ref(false)
const message = ref('')
const error = ref('')
const form = reactive({
  siteName: props.status.site?.name ?? '',
  siteUrl: props.status.site?.url ?? window.location.origin,
  adminEmail: props.status.site?.adminMail ?? '',
  timezone: props.status.site?.timezone ?? 'Asia/Shanghai',
  adminUsername: '',
  adminPassword: '',
})

async function submit() {
  submitting.value = true
  message.value = ''
  error.value = ''
  try {
    if (!initialized) {
      await postJSON('/api/v1/setup', form)
      window.location.replace('/admin')
      return
    }
    await putJSON<Site>('/api/v1/settings/site', {
      name: form.siteName,
      url: form.siteUrl,
      adminMail: form.adminEmail,
      timezone: form.timezone,
    })
    message.value = '设置已保存'
  } catch (reason) {
    error.value = reason instanceof ApiError ? reason.message : '保存失败'
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <main class="setup-shell">
    <section class="setup-intro">
      <a href="/user" class="brand"><span class="brand-mark">T</span> Typecho Next</a>
      <div class="intro-copy">
        <p class="eyebrow">FIRST RUN</p>
        <h1>{{ initialized ? '站点设置' : '把你的站点准备好' }}</h1>
        <p>{{ initialized ? '集中管理站点信息，后续模块会在这里逐步接入。' : '新架构已经就绪。完成一次设置后，管理端和用户端就会开放。' }}</p>
      </div>
      <div class="intro-foot"><span class="dot"></span> Go API · Vue 3 · TypeScript</div>
    </section>
    <section class="setup-card">
      <div class="card-heading">
        <div>
          <p class="eyebrow">{{ initialized ? 'SITE SETTINGS' : 'INITIAL SETUP' }}</p>
          <h2>{{ initialized ? '站点信息' : '创建管理员账户' }}</h2>
        </div>
        <span class="step">{{ initialized ? '•••' : '01 / 01' }}</span>
      </div>
      <form @submit.prevent="submit">
        <label>站点名称<input v-model="form.siteName" required maxlength="80" placeholder="例如：我的新博客" /></label>
        <label>站点地址<input v-model="form.siteUrl" required type="url" placeholder="https://example.com" /></label>
        <div class="form-grid">
          <label>管理员邮箱<input v-model="form.adminEmail" required type="email" placeholder="you@example.com" /></label>
          <label>时区<select v-model="form.timezone"><option>Asia/Shanghai</option><option>Asia/Tokyo</option><option>UTC</option><option>America/Los_Angeles</option></select></label>
        </div>
        <template v-if="!initialized">
          <div class="form-divider"><span>管理员登录</span></div>
          <div class="form-grid">
            <label>用户名<input v-model="form.adminUsername" required minlength="3" maxlength="32" pattern="[A-Za-z0-9_-]+" placeholder="admin" /></label>
            <label>密码<input v-model="form.adminPassword" required minlength="8" type="password" placeholder="至少 8 位" /></label>
          </div>
        </template>
        <p v-if="error" class="form-error">{{ error }}</p>
        <p v-if="message" class="form-success">{{ message }}</p>
        <button class="button button-primary button-wide" :disabled="submitting">{{ submitting ? '保存中…' : initialized ? '保存设置' : '完成设置并进入管理端' }}</button>
      </form>
      <p class="card-note">数据保存在服务端数据目录中，密码经过单向哈希处理。</p>
    </section>
  </main>
</template>
