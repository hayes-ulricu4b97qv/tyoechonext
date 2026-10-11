<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { api, ApiError, postJSON, putJSON } from '../api'
import { getReading, updateReading } from '../api/settings'
import type { AuditEvent, AuditPage, ReadingSettings, Site, Status, UserRecord } from '../types'

const props = defineProps<{ status: Status }>()
const initialized = props.status.initialized
const submitting = ref(false)
const message = ref('')
const error = ref('')
const accountSubmitting = ref(false)
const accountMessage = ref('')
const accountError = ref('')
const canManageUsers = ref(false)
const users = ref<UserRecord[]>([])
const usersLoading = ref(false)
const userSubmitting = ref(false)
const usersMessage = ref('')
const usersError = ref('')
const auditEvents = ref<AuditEvent[]>([])
const auditLoading = ref(false)
const auditError = ref('')
const form = reactive({
  siteName: props.status.site?.name ?? '',
  siteUrl: props.status.site?.url ?? window.location.origin,
  adminEmail: props.status.site?.adminMail ?? '',
  timezone: props.status.site?.timezone ?? 'Asia/Shanghai',
  description: props.status.site?.description ?? '',
  keywords: props.status.site?.keywords ?? '',
  adminUsername: '',
  adminPassword: '',
})
const accountForm = reactive({ email: props.status.site?.adminMail ?? '', currentPassword: '', newPassword: '' })
const userForm = reactive({ username: '', email: '', password: '', role: 'editor' as 'editor' | 'moderator' })
const readingForm = reactive<ReadingSettings>({
  postsPerPage: props.status.reading?.postsPerPage ?? 10,
  commentsEnabled: props.status.reading?.commentsEnabled ?? true,
  commentsRequireModeration: props.status.reading?.commentsRequireModeration ?? true,
  commentsThreaded: props.status.reading?.commentsThreaded ?? true,
})
const readingSubmitting = ref(false)
const readingMessage = ref('')
const readingError = ref('')

async function loadAccount() {
  if (!initialized) return
  try {
    const response = await api<{ user: { email: string; role?: string } }>('/api/v1/auth/me')
    accountForm.email = response.user.email
    canManageUsers.value = response.user.role === 'owner'
    if (canManageUsers.value) {
      void loadUsers()
      void loadAudit()
      void loadReading()
    }
  } catch (reason) {
    if (!(reason instanceof ApiError && reason.status === 401)) {
      accountError.value = reason instanceof ApiError ? reason.message : '无法读取账户信息'
    }
  }
}

async function loadAudit() {
  auditLoading.value = true
  auditError.value = ''
  try {
    const response = await api<AuditPage>('/api/v1/audit?page=1&pageSize=20')
    auditEvents.value = response.items
  } catch (reason) {
    auditError.value = reason instanceof ApiError ? reason.message : '无法读取审计记录'
  } finally {
    auditLoading.value = false
  }
}

async function loadReading() {
  try {
    Object.assign(readingForm, await getReading())
  } catch (reason) {
    readingError.value = reason instanceof ApiError ? reason.message : '无法读取阅读设置'
  }
}

async function saveReading() {
  readingSubmitting.value = true
  readingMessage.value = ''
  readingError.value = ''
  try {
    Object.assign(readingForm, await updateReading({ ...readingForm }))
    readingMessage.value = '阅读与评论设置已保存'
  } catch (reason) {
    readingError.value = reason instanceof ApiError ? reason.message : '保存阅读设置失败'
  } finally {
    readingSubmitting.value = false
  }
}

async function loadUsers() {
  usersLoading.value = true
  usersError.value = ''
  try {
    const response = await api<{ items: UserRecord[] }>('/api/v1/settings/users')
    users.value = response.items
  } catch (reason) {
    usersError.value = reason instanceof ApiError ? reason.message : '无法读取用户列表'
  } finally {
    usersLoading.value = false
  }
}

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
      description: form.description,
      keywords: form.keywords,
    })
    message.value = '设置已保存'
  } catch (reason) {
    error.value = reason instanceof ApiError ? reason.message : '保存失败'
  } finally {
    submitting.value = false
  }
}

async function submitAccount() {
  accountSubmitting.value = true
  accountMessage.value = ''
  accountError.value = ''
  try {
    await putJSON('/api/v1/settings/account', accountForm)
    accountForm.currentPassword = ''
    accountForm.newPassword = ''
    accountMessage.value = '账户信息已保存'
  } catch (reason) {
    accountError.value = reason instanceof ApiError ? reason.message : '保存账户信息失败'
  } finally {
    accountSubmitting.value = false
  }
}

async function createUser() {
  userSubmitting.value = true
  usersMessage.value = ''
  usersError.value = ''
  try {
    await postJSON('/api/v1/settings/users', userForm)
    Object.assign(userForm, { username: '', email: '', password: '', role: 'editor' })
    usersMessage.value = '用户已创建'
    await loadUsers()
  } catch (reason) {
    usersError.value = reason instanceof ApiError ? reason.message : '创建用户失败'
  } finally {
    userSubmitting.value = false
  }
}

async function updateUser(user: UserRecord) {
  usersError.value = ''
  try {
    await putJSON(`/api/v1/settings/users/${encodeURIComponent(user.username)}`, { email: user.email, role: user.role, active: user.active, password: user.password?.trim() || undefined })
    user.password = ''
    usersMessage.value = '用户信息已保存'
  } catch (reason) {
    usersError.value = reason instanceof ApiError ? reason.message : '保存用户失败'
    await loadUsers()
  }
}

async function removeUser(user: UserRecord) {
  if (!window.confirm(`确定删除用户“${user.username}”吗？`)) return
  usersError.value = ''
  try {
    await api(`/api/v1/settings/users/${encodeURIComponent(user.username)}`, { method: 'DELETE' })
    usersMessage.value = '用户已删除'
    await loadUsers()
  } catch (reason) {
    usersError.value = reason instanceof ApiError ? reason.message : '删除用户失败'
  }
}

onMounted(() => {
  void loadAccount()
})
</script>

<template>
  <main class="setup-shell">
    <section class="setup-intro">
      <a href="/user" class="brand"><span class="brand-mark">T</span> Typecho Next</a>
      <div class="intro-copy">
        <p class="eyebrow">FIRST RUN</p>
        <h1>{{ initialized ? '站点设置' : '把你的站点准备好' }}</h1>
        <p>{{ initialized ? '站点信息、阅读和评论规则都由后端校验后保存。' : '新架构已经就绪。完成一次设置后，管理端和用户端就会开放。' }}</p>
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
      <form v-if="!initialized || canManageUsers" @submit.prevent="submit">
        <label>站点名称<input v-model="form.siteName" required maxlength="80" placeholder="例如：我的新博客" /></label>
        <label>站点地址<input v-model="form.siteUrl" required type="url" placeholder="https://example.com" /></label>
        <template v-if="initialized">
          <label>站点描述<textarea v-model="form.description" maxlength="2000" rows="3" placeholder="用一两句话介绍这个站点"></textarea></label>
          <label>关键词<input v-model="form.keywords" maxlength="500" placeholder="用逗号分隔，例如：笔记, Go" /></label>
        </template>
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
      <section v-if="initialized && canManageUsers" class="settings-account">
        <div class="form-divider"><span>管理员账户</span></div>
        <form @submit.prevent="submitAccount">
          <label>账户邮箱<input v-model="accountForm.email" required type="email" autocomplete="email" placeholder="you@example.com" /></label>
          <label>当前密码<input v-model="accountForm.currentPassword" type="password" autocomplete="current-password" placeholder="修改密码时填写" /></label>
          <label>新密码<input v-model="accountForm.newPassword" minlength="8" type="password" autocomplete="new-password" placeholder="留空则不修改密码" /></label>
          <p v-if="accountError" class="form-error">{{ accountError }}</p>
          <p v-if="accountMessage" class="form-success">{{ accountMessage }}</p>
          <button class="button button-dark button-wide" :disabled="accountSubmitting">{{ accountSubmitting ? '保存中…' : '保存账户信息' }}</button>
        </form>
      </section>
      <section v-if="initialized && canManageUsers" class="settings-reading">
        <div class="form-divider"><span>阅读与评论</span></div>
        <form @submit.prevent="saveReading">
          <label>每页文章数<input v-model.number="readingForm.postsPerPage" required type="number" min="1" max="50" /></label>
          <label class="check-row"><input v-model="readingForm.commentsEnabled" type="checkbox" />开放评论</label>
          <label class="check-row"><input v-model="readingForm.commentsRequireModeration" type="checkbox" />新评论需要审核</label>
          <label class="check-row"><input v-model="readingForm.commentsThreaded" type="checkbox" />允许回复评论，最多三层</label>
          <p v-if="readingError" class="form-error">{{ readingError }}</p>
          <p v-if="readingMessage" class="form-success">{{ readingMessage }}</p>
          <button class="button button-dark button-wide" :disabled="readingSubmitting">{{ readingSubmitting ? '保存中…' : '保存阅读与评论设置' }}</button>
        </form>
      </section>
      <section v-if="initialized && canManageUsers" class="settings-users">
        <div class="form-divider"><span>用户与权限</span></div>
        <form class="user-create-form" @submit.prevent="createUser">
          <div class="form-grid"><label>用户名<input v-model="userForm.username" required minlength="3" maxlength="32" pattern="[A-Za-z0-9_-]+" placeholder="editor" /></label><label>邮箱<input v-model="userForm.email" required type="email" placeholder="editor@example.com" /></label></div>
          <div class="form-grid"><label>初始密码<input v-model="userForm.password" required minlength="8" type="password" placeholder="至少 8 位" /></label><label>角色<select v-model="userForm.role"><option value="editor">编辑</option><option value="moderator">审核员</option></select></label></div>
          <button class="button button-dark" :disabled="userSubmitting">{{ userSubmitting ? '创建中…' : '创建用户' }}</button>
        </form>
        <p v-if="usersError" class="form-error">{{ usersError }}</p>
        <p v-if="usersMessage" class="form-success">{{ usersMessage }}</p>
        <div v-if="usersLoading" class="user-state">正在加载用户…</div>
        <div v-else class="user-list">
          <div v-for="managedUser in users" :key="managedUser.id" class="user-row">
            <div class="user-row-main"><strong>{{ managedUser.username }}</strong><span>{{ managedUser.email }}</span></div>
            <select v-model="managedUser.role" :disabled="managedUser.role === 'owner'" @change="updateUser(managedUser)"><option value="owner">所有者</option><option value="editor">编辑</option><option value="moderator">审核员</option></select>
            <input v-if="managedUser.role !== 'owner'" v-model="managedUser.password" class="user-password" type="password" minlength="8" placeholder="重置密码" @keyup.enter="updateUser(managedUser)" />
            <label class="user-active"><input v-model="managedUser.active" type="checkbox" :disabled="managedUser.role === 'owner'" @change="updateUser(managedUser)" />启用</label>
            <button v-if="managedUser.role !== 'owner'" class="text-button" @click="updateUser(managedUser)">保存</button>
            <button v-if="managedUser.role !== 'owner'" class="text-button danger" @click="removeUser(managedUser)">删除</button>
          </div>
        </div>
      </section>
      <section v-if="initialized && canManageUsers" class="settings-audit">
        <div class="form-divider"><span>操作记录</span></div>
        <p v-if="auditError" class="form-error">{{ auditError }}</p>
        <div v-if="auditLoading" class="user-state">正在加载操作记录…</div>
        <div v-else-if="auditEvents.length === 0" class="user-state">还没有操作记录</div>
        <div v-else class="audit-list">
          <article v-for="event in auditEvents" :key="event.id" class="audit-row">
            <div><strong>{{ event.action }}</strong><span>{{ event.resourceType }}{{ event.resourceId ? ` · ${event.resourceId}` : '' }}</span></div>
            <div><span>{{ event.actorUsername }}</span><time>{{ new Date(event.createdAt).toLocaleString('zh-CN') }}</time></div>
          </article>
        </div>
      </section>
      <p v-if="initialized && !canManageUsers" class="form-error">只有所有者可以修改站点和账户设置。</p>
      <p class="card-note">数据保存在服务端数据目录中，密码经过单向哈希处理。</p>
    </section>
  </main>
</template>
