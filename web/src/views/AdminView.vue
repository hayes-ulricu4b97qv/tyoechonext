<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ApiError } from '../api'
import { getCurrentUser, login as loginRequest, logout as logoutRequest } from '../api/auth'
import {
  createTerm as createTermRequest,
  deleteComment,
  deleteMedia,
  deletePage,
  deletePost,
  deleteTerm,
  listMedia,
  listPages,
  listPendingComments,
  listPosts,
  listTerms,
  moderateComment,
  savePage as savePageRequest,
  savePost,
  uploadMedia as uploadMediaFile,
} from '../api/content'
import { getDashboardStats } from '../api/settings'
import type { Comment, CommentPage, Media, MediaPage, Page, PagePage, Post, PostPage, Stats, Status, Term, TermPage, User } from '../types'

defineProps<{ status: Status }>()

const user = ref<User | null>(null)
const stats = ref<Stats>({ posts: 0, publishedPosts: 0, drafts: 0, comments: 0, pendingComments: 0 })
const posts = ref<Post[]>([])
const pages = ref<Page[]>([])
const pendingComments = ref<Comment[]>([])
const media = ref<Media[]>([])
const terms = ref<Term[]>([])
const loading = ref(true)
const error = ref('')
const loginForm = reactive({ username: '', password: '' })
const loginError = ref('')
const loginLoading = ref(false)
const showEditor = ref(false)
const editor = reactive({ title: '', slug: '', excerpt: '', content: '', categories: '', tags: '', status: 'draft' as 'draft' | 'published' })
const editorError = ref('')
const editingId = ref<string | null>(null)
const showPageEditor = ref(false)
const pageEditor = reactive({ title: '', slug: '', excerpt: '', content: '', status: 'draft' as 'draft' | 'published' })
const pageEditorError = ref('')
const editingPageId = ref<string | null>(null)
const commentError = ref('')
const commentActionId = ref<string | null>(null)
const mediaError = ref('')
const mediaUploading = ref(false)
const mediaActionId = ref<string | null>(null)
const termForm = reactive({ name: '', kind: 'category' as 'category' | 'tag' })
const termError = ref('')
const termSubmitting = ref(false)
const termActionId = ref<string | null>(null)
const canEditContent = computed(() => user.value?.role === 'owner' || user.value?.role === 'editor')
const canModerateComments = computed(() => user.value?.role === 'owner' || user.value?.role === 'moderator')

async function loadDashboard() {
  loading.value = true
  error.value = ''
  try {
    const me = await getCurrentUser()
    user.value = me.user
    const canEdit = me.user.role === 'owner' || me.user.role === 'editor'
    const canModerate = me.user.role === 'owner' || me.user.role === 'moderator'
    const [summary, postResponse, pageResponse, commentResponse, mediaResponse, termResponse] = await Promise.all([
      getDashboardStats(),
      listPosts({ includeDraft: true, page: 1, pageSize: 100 }),
      canEdit ? listPages(true, 1, 100) : Promise.resolve({ items: [], total: 0, page: 1, pageSize: 100 }),
      canModerate ? listPendingComments(1, 100) : Promise.resolve({ items: [], total: 0, page: 1, pageSize: 100 }),
      canEdit ? listMedia(1, 100) : Promise.resolve({ items: [], total: 0, page: 1, pageSize: 100 }),
      canEdit ? listTerms(true, 1, 100) : Promise.resolve({ items: [], total: 0, page: 1, pageSize: 100 }),
    ])
    stats.value = summary
    posts.value = postResponse.items
    pages.value = pageResponse.items
    pendingComments.value = commentResponse.items
    media.value = mediaResponse.items
    terms.value = termResponse.items
  } catch (reason) {
    if (!(reason instanceof ApiError && reason.status === 401)) {
      error.value = reason instanceof ApiError ? reason.message : '管理数据加载失败'
    }
  } finally {
    loading.value = false
  }
}

async function login() {
  loginLoading.value = true
  loginError.value = ''
  try {
    const response = await loginRequest(loginForm)
    user.value = response.user
    await loadDashboard()
  } catch (reason) {
    loginError.value = reason instanceof ApiError ? reason.message : '登录失败'
  } finally {
    loginLoading.value = false
  }
}

async function logout() {
  await logoutRequest()
  user.value = null
}

function startEdit(post: Post) {
  editingId.value = post.id
  Object.assign(editor, {
    title: post.title,
    slug: post.slug,
    excerpt: post.excerpt,
    content: post.content,
    categories: post.categories?.map((term) => term.name).join(', ') ?? '',
    tags: post.tags?.map((term) => term.name).join(', ') ?? '',
    status: post.status,
  })
  editorError.value = ''
  showEditor.value = true
}

function cancelEditor() {
  editingId.value = null
  showEditor.value = false
  Object.assign(editor, { title: '', slug: '', excerpt: '', content: '', categories: '', tags: '', status: 'draft' })
  editorError.value = ''
}

async function publish() {
  editorError.value = ''
  try {
    const payload = {
      title: editor.title,
      slug: editor.slug,
      excerpt: editor.excerpt,
      content: editor.content,
      status: editor.status,
      categories: splitTerms(editor.categories),
      tags: splitTerms(editor.tags),
    }
    if (editingId.value) {
    await savePost(editingId.value, payload)
    cancelEditor()
    await loadDashboard()
  } catch (reason) {
    editorError.value = reason instanceof ApiError ? reason.message : '发布失败'
  }
}

function startPageEdit(page: Page) {
  editingPageId.value = page.id
  Object.assign(pageEditor, { title: page.title, slug: page.slug, excerpt: page.excerpt, content: page.content, status: page.status })
  pageEditorError.value = ''
  showPageEditor.value = true
}

function cancelPageEditor() {
  editingPageId.value = null
  showPageEditor.value = false
  Object.assign(pageEditor, { title: '', slug: '', excerpt: '', content: '', status: 'draft' })
  pageEditorError.value = ''
}

async function savePage() {
  pageEditorError.value = ''
  try {
    const payload = { title: pageEditor.title, slug: pageEditor.slug, excerpt: pageEditor.excerpt, content: pageEditor.content, status: pageEditor.status }
    if (editingPageId.value) {
    await savePageRequest(editingPageId.value, payload)
    cancelPageEditor()
    await loadDashboard()
  } catch (reason) {
    pageEditorError.value = reason instanceof ApiError ? reason.message : '保存页面失败'
  }
}

async function removePage(page: Page) {
  if (!window.confirm(`确定删除页面“${page.title}”吗？`)) return
  try {
    await deletePage(page.id)
    await loadDashboard()
  } catch (reason) {
    error.value = reason instanceof ApiError ? reason.message : '删除页面失败'
  }
}

function splitTerms(value: string) {
  return [...new Set(value.split(',').map((term) => term.trim()).filter(Boolean))]
}

function postPreviewURL(post: Post) {
  return `/user/${encodeURIComponent(post.slug)}`
}

function pagePreviewURL(page: Page) {
  return `/page/${encodeURIComponent(page.slug)}`
}

async function removePost(post: Post) {
  if (!window.confirm(`确定删除“${post.title}”吗？`)) return
  try {
    await deletePost(post.id)
    await loadDashboard()
  } catch (reason) {
    error.value = reason instanceof ApiError ? reason.message : '删除失败'
  }
}

async function moderateComment(comment: Comment, status: 'approved' | 'spam') {
  commentActionId.value = comment.id
  commentError.value = ''
  try {
    await moderateComment(comment.id, status)
    await loadDashboard()
  } catch (reason) {
    commentError.value = reason instanceof ApiError ? reason.message : '更新评论失败'
  } finally {
    commentActionId.value = null
  }
}

async function removeComment(comment: Comment) {
  if (!window.confirm(`确定删除来自“${comment.authorName}”的评论吗？`)) return
  commentActionId.value = comment.id
  commentError.value = ''
  try {
    await deleteComment(comment.id)
    await loadDashboard()
  } catch (reason) {
    commentError.value = reason instanceof ApiError ? reason.message : '删除评论失败'
  } finally {
    commentActionId.value = null
  }
}

async function uploadMedia(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  mediaUploading.value = true
  mediaError.value = ''
  try {
    await uploadMediaFile(file)
    input.value = ''
    await loadDashboard()
  } catch (reason) {
    mediaError.value = reason instanceof ApiError ? reason.message : '上传媒体失败'
  } finally {
    mediaUploading.value = false
  }
}

async function removeMedia(asset: Media) {
  if (!window.confirm(`确定删除“${asset.filename}”吗？`)) return
  mediaActionId.value = asset.id
  mediaError.value = ''
  try {
    await deleteMedia(asset.id)
    await loadDashboard()
  } catch (reason) {
    mediaError.value = reason instanceof ApiError ? reason.message : '删除媒体失败'
  } finally {
    mediaActionId.value = null
  }
}

async function createTerm() {
  termSubmitting.value = true
  termError.value = ''
  try {
    await createTermRequest(termForm)
    termForm.name = ''
    await loadDashboard()
  } catch (reason) {
    termError.value = reason instanceof ApiError ? reason.message : '保存分类/标签失败'
  } finally {
    termSubmitting.value = false
  }
}

async function removeTerm(term: Term) {
  if (!window.confirm(`确定删除“${term.name}”吗？`)) return
  termActionId.value = term.id
  termError.value = ''
  try {
    await deleteTerm(term.id)
    await loadDashboard()
  } catch (reason) {
    termError.value = reason instanceof ApiError ? reason.message : '删除分类/标签失败'
  } finally {
    termActionId.value = null
  }
}

onMounted(async () => {
  await loadDashboard()
})
</script>

<template>
  <div class="admin-shell">
    <aside class="admin-sidebar">
      <a class="brand" href="/user"><span class="brand-mark">T</span><span>Typecho Next</span></a>
      <div class="side-label">WORKSPACE</div>
      <nav class="side-nav">
        <a class="selected" href="/admin"><span>▦</span>概览</a>
        <a href="/admin#posts"><span>◫</span>文章</a>
        <a v-if="user?.role === 'owner'" href="/setting"><span>⚙</span>设置</a>
      </nav>
      <div class="side-bottom">
        <div class="status-pill"><span class="status-dot"></span>服务运行中</div>
        <a href="/user">查看用户端 ↗</a>
      </div>
    </aside>
    <main class="admin-main">
      <header class="admin-topbar">
        <div><p class="eyebrow">ADMIN / OVERVIEW</p><h1>欢迎回来<span v-if="user">，{{ user.username }}</span></h1></div>
        <div class="top-actions"><a v-if="user?.role === 'owner'" href="/setting" class="icon-button" title="设置">⚙</a><button v-if="user" class="avatar" @click="logout">{{ user.username.slice(0, 1).toUpperCase() }}</button></div>
      </header>

      <div v-if="!user" class="login-panel">
        <div class="login-copy"><span class="login-mark">↗</span><h2>进入管理端</h2><p>使用首次设置时创建的管理员账户登录。</p></div>
        <form @submit.prevent="login">
          <label>用户名<input v-model="loginForm.username" required autocomplete="username" placeholder="admin" /></label>
          <label>密码<input v-model="loginForm.password" required type="password" autocomplete="current-password" placeholder="••••••••" /></label>
          <p v-if="loginError" class="form-error">{{ loginError }}</p>
          <button class="button button-primary button-wide" :disabled="loginLoading">{{ loginLoading ? '登录中…' : '登录管理端' }}</button>
        </form>
      </div>

      <template v-else>
        <div class="stats-grid">
          <div class="stat-card"><span>全部文章</span><strong>{{ stats.posts }}</strong><small>累计内容</small></div>
          <div class="stat-card stat-green"><span>已发布</span><strong>{{ stats.publishedPosts }}</strong><small>对外可见</small></div>
          <div class="stat-card stat-yellow"><span>草稿</span><strong>{{ stats.drafts }}</strong><small>等待完善</small></div>
          <div class="stat-card stat-blue"><span>待审核评论</span><strong>{{ stats.pendingComments }}</strong><small>需要处理</small></div>
        </div>

        <section v-if="canModerateComments && pendingComments.length > 0" class="admin-section comments-section">
          <div class="section-heading"><div><p class="eyebrow">MODERATION</p><h2>待审核评论</h2></div><span class="section-count">{{ pendingComments.length }} 条</span></div>
          <p v-if="commentError" class="form-error">{{ commentError }}</p>
          <div class="admin-comments">
            <article v-for="comment in pendingComments" :key="comment.id" class="admin-comment-row">
              <div class="comment-row-content">
                <div class="comment-row-meta"><strong>{{ comment.authorName }}</strong><span v-if="comment.authorEmail">{{ comment.authorEmail }}</span><time>{{ new Date(comment.createdAt).toLocaleDateString('zh-CN') }}</time></div>
                <p>{{ comment.content }}</p>
                <small v-if="comment.postTitle">文章：{{ comment.postTitle }}</small>
              </div>
              <div class="row-actions"><button class="text-button" :disabled="commentActionId === comment.id" @click="moderateComment(comment, 'approved')">通过</button><button class="text-button danger" :disabled="commentActionId === comment.id" @click="moderateComment(comment, 'spam')">垃圾</button><button class="text-button danger" :disabled="commentActionId === comment.id" @click="removeComment(comment)">删除</button></div>
            </article>
          </div>
        </section>

        <section v-if="canEditContent" class="admin-section media-section">
          <div class="section-heading"><div><p class="eyebrow">MEDIA</p><h2>媒体</h2></div><label class="button button-primary media-upload"><input type="file" accept="image/jpeg,image/png,image/gif,image/webp" :disabled="mediaUploading" @change="uploadMedia" />{{ mediaUploading ? '上传中…' : '+ 上传图片' }}</label></div>
          <p v-if="mediaError" class="form-error">{{ mediaError }}</p>
          <div v-if="media.length === 0" class="empty-state media-empty">还没有上传媒体</div>
          <div v-else class="media-grid">
            <article v-for="asset in media" :key="asset.id" class="media-card">
              <a :href="asset.url" target="_blank" rel="noreferrer"><img :src="asset.url" :alt="asset.filename" loading="lazy" /></a>
              <div class="media-card-info"><strong :title="asset.filename">{{ asset.filename }}</strong><span>{{ Math.ceil(asset.size / 1024) }} KB</span></div>
              <button class="text-button danger" :disabled="mediaActionId === asset.id" @click="removeMedia(asset)">删除</button>
            </article>
          </div>
        </section>

        <section v-if="canEditContent" class="admin-section taxonomy-section">
          <div class="section-heading"><div><p class="eyebrow">TAXONOMY</p><h2>分类与标签</h2></div><span class="section-count">{{ terms.length }} 个</span></div>
          <form class="taxonomy-form" @submit.prevent="createTerm">
            <input v-model="termForm.name" required maxlength="60" placeholder="名称" />
            <select v-model="termForm.kind"><option value="category">分类</option><option value="tag">标签</option></select>
            <button class="button button-primary" :disabled="termSubmitting">{{ termSubmitting ? '保存中…' : '新增' }}</button>
          </form>
          <p v-if="termError" class="form-error">{{ termError }}</p>
          <div v-if="terms.length === 0" class="empty-state taxonomy-empty">还没有分类或标签</div>
          <div v-else class="taxonomy-list">
            <div v-for="term in terms" :key="term.id" class="taxonomy-row">
              <div><strong>{{ term.name }}</strong><small>{{ term.kind === 'category' ? '分类' : '标签' }} · {{ term.postCount }} 篇已发布文章</small></div>
              <button class="text-button danger" :disabled="termActionId === term.id" @click="removeTerm(term)">删除</button>
            </div>
          </div>
        </section>

        <section v-if="canEditContent" class="admin-section page-section" id="pages">
          <div class="section-heading"><div><p class="eyebrow">PAGES</p><h2>独立页面</h2></div><button class="button button-primary" @click="showPageEditor ? cancelPageEditor() : (showPageEditor = true)">{{ showPageEditor ? '关闭编辑器' : '+ 新建页面' }}</button></div>
          <form v-if="showPageEditor" class="editor-card" @submit.prevent="savePage">
            <input v-model="pageEditor.title" class="editor-title" required placeholder="页面标题" />
            <label class="editor-slug">固定链接<input v-model="pageEditor.slug" placeholder="留空则按标题生成" /></label>
            <textarea v-model="pageEditor.excerpt" rows="2" placeholder="页面摘要（可选）"></textarea>
            <textarea v-model="pageEditor.content" rows="8" required placeholder="页面内容，支持 Markdown…"></textarea>
            <div class="editor-actions"><select v-model="pageEditor.status"><option value="draft">保存为草稿</option><option value="published">立即发布</option></select><p v-if="pageEditorError" class="form-error">{{ pageEditorError }}</p><button class="button button-dark">{{ editingPageId ? '保存修改' : '保存页面' }}</button></div>
          </form>
          <div v-if="pages.length === 0" class="empty-state page-empty">还没有独立页面</div>
          <div v-else class="admin-posts">
            <div v-for="page in pages" :key="page.id" class="admin-post-row"><div><span class="post-status" :class="page.status">{{ page.status === 'published' ? '已发布' : '草稿' }}</span><div><strong>{{ page.title }}</strong><small>/page/{{ page.slug }}</small></div></div><time>{{ new Date(page.updatedAt).toLocaleDateString('zh-CN') }}</time><div class="row-actions"><button class="text-button" @click="startPageEdit(page)">编辑</button><a class="text-button" :href="pagePreviewURL(page)" target="_blank" rel="noreferrer">{{ page.status === 'published' ? '查看' : '预览' }}</a><button class="text-button danger" @click="removePage(page)">删除</button></div></div>
          </div>
        </section>

        <section class="admin-section" id="posts">
          <div class="section-heading"><div><p class="eyebrow">CONTENT</p><h2>文章</h2></div><button v-if="canEditContent" class="button button-primary" @click="showEditor ? cancelEditor() : (showEditor = true)">{{ showEditor ? '关闭编辑器' : '+ 写文章' }}</button></div>
          <form v-if="showEditor && canEditContent" class="editor-card" @submit.prevent="publish">
            <input v-model="editor.title" class="editor-title" required placeholder="文章标题" />
            <label class="editor-slug">固定链接<input v-model="editor.slug" placeholder="留空则保留现有地址或按标题生成" /></label>
            <textarea v-model="editor.excerpt" rows="2" placeholder="一句话摘要（可选）"></textarea>
            <textarea v-model="editor.content" rows="8" required placeholder="开始写作…"></textarea>
            <div class="form-grid"><label>分类<input v-model="editor.categories" placeholder="产品, 随笔" /></label><label>标签<input v-model="editor.tags" placeholder="Go, Vue, 重构" /></label></div>
            <div class="editor-actions"><select v-model="editor.status"><option value="draft">保存为草稿</option><option value="published">立即发布</option></select><p v-if="editorError" class="form-error">{{ editorError }}</p><button class="button button-dark">{{ editingId ? '保存修改' : '保存文章' }}</button></div>
          </form>
          <div v-if="loading" class="empty-state">正在加载…</div>
          <div v-else-if="posts.length === 0" class="empty-state"><strong>开始你的第一篇文章</strong><span>文章会保存在新的内容服务中。</span></div>
          <div v-else class="admin-posts">
            <div v-for="post in posts" :key="post.id" class="admin-post-row"><div><span class="post-status" :class="post.status">{{ post.status === 'published' ? '已发布' : '草稿' }}</span><div><strong>{{ post.title }}</strong><small v-if="post.categories?.length || post.tags?.length">{{ [...(post.categories ?? []), ...(post.tags ?? [])].map((term) => term.name).join(' · ') }}</small></div></div><time>{{ new Date(post.updatedAt).toLocaleDateString('zh-CN') }}</time><div v-if="canEditContent" class="row-actions"><button class="text-button" @click="startEdit(post)">编辑</button><a class="text-button" :href="postPreviewURL(post)" target="_blank" rel="noreferrer">{{ post.status === 'published' ? '查看' : '预览' }}</a><button class="text-button danger" @click="removePost(post)">删除</button></div></div>
          </div>
        </section>
      </template>
    </main>
  </div>
</template>
