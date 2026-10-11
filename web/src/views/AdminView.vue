<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { api, ApiError, postJSON, putJSON } from '../api'
import type { Post, PostPage, Stats, Status, User } from '../types'

defineProps<{ status: Status }>()
const user = ref<User | null>(null)
const stats = ref<Stats>({ posts: 0, publishedPosts: 0, drafts: 0 })
const posts = ref<Post[]>([])
const loading = ref(true)
const error = ref('')
const loginForm = reactive({ username: '', password: '' })
const loginError = ref('')
const loginLoading = ref(false)
const showEditor = ref(false)
const editor = reactive({ title: '', excerpt: '', content: '', status: 'draft' as 'draft' | 'published' })
const editorError = ref('')
const editingId = ref<string | null>(null)

async function loadDashboard() {
  loading.value = true
  try {
    const me = await api<{ user: User }>('/api/v1/auth/me')
    user.value = me.user
    const [summary, postResponse] = await Promise.all([
      api<Stats>('/api/v1/dashboard/stats'),
      api<PostPage>('/api/v1/content/posts?includeDraft=true&page=1&pageSize=100'),
    ])
    stats.value = summary
    posts.value = postResponse.items
  } catch (reason) {
    if (!(reason instanceof ApiError && reason.status === 401)) error.value = reason instanceof ApiError ? reason.message : '管理数据加载失败'
  } finally {
    loading.value = false
  }
}

async function login() {
  loginLoading.value = true
  loginError.value = ''
  try {
    const response = await postJSON<{ user: User }>('/api/v1/auth/login', loginForm)
    user.value = response.user
    await loadDashboard()
  } catch (reason) {
    loginError.value = reason instanceof ApiError ? reason.message : '登录失败'
  } finally {
    loginLoading.value = false
  }
}

async function logout() {
  await api('/api/v1/auth/logout', { method: 'POST' })
  user.value = null
}

function startEdit(post: Post) {
  editingId.value = post.id
  Object.assign(editor, { title: post.title, excerpt: post.excerpt, content: post.content, status: post.status })
  editorError.value = ''
  showEditor.value = true
}

function cancelEditor() {
  editingId.value = null
  showEditor.value = false
  Object.assign(editor, { title: '', excerpt: '', content: '', status: 'draft' })
  editorError.value = ''
}

async function publish() {
  editorError.value = ''
  try {
    if (editingId.value) {
      await putJSON(`/api/v1/content/posts/${editingId.value}`, editor)
    } else {
      await postJSON('/api/v1/content/posts', editor)
    }
    cancelEditor()
    await loadDashboard()
  } catch (reason) {
    editorError.value = reason instanceof ApiError ? reason.message : '发布失败'
  }
}

async function removePost(post: Post) {
  if (!window.confirm(`确定删除“${post.title}”吗？`)) return
  try {
    await api(`/api/v1/content/posts/${post.id}`, { method: 'DELETE' })
    await loadDashboard()
  } catch (reason) {
    error.value = reason instanceof ApiError ? reason.message : '删除失败'
  }
}

onMounted(async () => {
  await loadDashboard()
})
</script>

<template>
  <div class="admin-shell">
    <aside class="admin-sidebar"><a class="brand" href="/user"><span class="brand-mark">T</span><span>Typecho Next</span></a><div class="side-label">WORKSPACE</div><nav class="side-nav"><a class="selected" href="/admin"><span>▦</span>概览</a><a href="/admin#posts"><span>◫</span>文章</a><a href="/setting"><span>⚙</span>设置</a></nav><div class="side-bottom"><div class="status-pill"><span class="status-dot"></span>服务运行中</div><a href="/user">查看用户端 ↗</a></div></aside>
    <main class="admin-main">
      <header class="admin-topbar"><div><p class="eyebrow">ADMIN / OVERVIEW</p><h1>欢迎回来<span v-if="user">，{{ user.username }}</span></h1></div><div class="top-actions"><a href="/setting" class="icon-button" title="设置">⚙</a><button v-if="user" class="avatar" @click="logout">{{ user.username.slice(0, 1).toUpperCase() }}</button></div></header>
      <div v-if="!user" class="login-panel"><div class="login-copy"><span class="login-mark">↗</span><h2>进入管理端</h2><p>使用首次设置时创建的管理员账户登录。</p></div><form @submit.prevent="login"><label>用户名<input v-model="loginForm.username" required autocomplete="username" placeholder="admin" /></label><label>密码<input v-model="loginForm.password" required type="password" autocomplete="current-password" placeholder="••••••••" /></label><p v-if="loginError" class="form-error">{{ loginError }}</p><button class="button button-primary button-wide" :disabled="loginLoading">{{ loginLoading ? '登录中…' : '登录管理端' }}</button></form></div>
      <template v-else><div class="stats-grid"><div class="stat-card"><span>全部文章</span><strong>{{ stats.posts }}</strong><small>累计内容</small></div><div class="stat-card stat-green"><span>已发布</span><strong>{{ stats.publishedPosts }}</strong><small>对外可见</small></div><div class="stat-card stat-yellow"><span>草稿</span><strong>{{ stats.drafts }}</strong><small>等待完善</small></div></div><section class="admin-section" id="posts"><div class="section-heading"><div><p class="eyebrow">CONTENT</p><h2>文章</h2></div><button class="button button-primary" @click="showEditor ? cancelEditor() : (showEditor = true)">{{ showEditor ? '关闭编辑器' : '+ 写文章' }}</button></div><form v-if="showEditor" class="editor-card" @submit.prevent="publish"><input v-model="editor.title" class="editor-title" required placeholder="文章标题" /><textarea v-model="editor.excerpt" rows="2" placeholder="一句话摘要（可选）"></textarea><textarea v-model="editor.content" rows="8" required placeholder="开始写作…"></textarea><div class="editor-actions"><select v-model="editor.status"><option value="draft">保存为草稿</option><option value="published">立即发布</option></select><p v-if="editorError" class="form-error">{{ editorError }}</p><button class="button button-dark">{{ editingId ? '保存修改' : '保存文章' }}</button></div></form><div v-if="loading" class="empty-state">正在加载…</div><div v-else-if="posts.length === 0" class="empty-state"><strong>开始你的第一篇文章</strong><span>文章会保存在新的内容服务中。</span></div><div v-else class="admin-posts"><div v-for="post in posts" :key="post.id" class="admin-post-row"><div><span class="post-status" :class="post.status">{{ post.status === 'published' ? '已发布' : '草稿' }}</span><strong>{{ post.title }}</strong></div><time>{{ new Date(post.updatedAt).toLocaleDateString('zh-CN') }}</time><div class="row-actions"><button class="text-button" @click="startEdit(post)">编辑</button><button class="text-button danger" @click="removePost(post)">删除</button></div></div></div></section></template>
    </main>
  </div>
</template>
