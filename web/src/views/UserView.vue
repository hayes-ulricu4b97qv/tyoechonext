<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api, ApiError } from '../api'
import type { Post, PostPage, Status } from '../types'

defineProps<{ status: Status }>()
const posts = ref<Post[]>([])
const selectedPost = ref<Post | null>(null)
const loading = ref(true)
const error = ref('')
const search = ref('')
const submittedSearch = ref('')
const page = ref(1)
const pageSize = 9
const total = ref(0)

async function loadPosts() {
  loading.value = true
  error.value = ''
  try {
    const query = new URLSearchParams({ page: String(page.value), pageSize: String(pageSize) })
    if (submittedSearch.value.trim()) query.set('q', submittedSearch.value.trim())
    const response = await api<PostPage>(`/api/v1/content/posts?${query.toString()}`)
    posts.value = response.items
    total.value = response.total
  } catch (reason) {
    error.value = reason instanceof ApiError ? reason.message : '文章加载失败'
  } finally {
    loading.value = false
  }
}

function submitSearch() {
  submittedSearch.value = search.value
  page.value = 1
  void loadPosts()
}

function changePage(nextPage: number) {
  if (nextPage < 1 || nextPage > totalPages.value || nextPage === page.value) return
  page.value = nextPage
  void loadPosts()
}

const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize)))

onMounted(async () => {
  const slug = window.location.pathname.match(/^\/user\/(.+)$/)?.[1]
  if (slug) {
    try {
      selectedPost.value = await api<Post>(`/api/v1/content/posts/${encodeURIComponent(slug)}`)
    } catch (reason) {
      error.value = reason instanceof ApiError ? reason.message : '文章加载失败'
    } finally {
      loading.value = false
    }
  } else {
    await loadPosts()
  }
})

</script>

<template>
  <div class="public-shell">
    <header class="public-nav">
      <a class="brand" href="/user"><span class="brand-mark">T</span> {{ status.site?.name ?? 'Typecho Next' }}</a>
      <nav><a href="/user" class="active">阅读</a><a href="/admin">管理</a></nav>
    </header>
    <main class="public-content">
      <section v-if="!selectedPost" class="hero-block"><p class="eyebrow">A NEW PUBLISHING CORE</p><h1>写下值得被看见的东西。</h1><p>这是全新 Go + Vue 架构的用户端。轻量、清晰，也给未来留出空间。</p></section>
      <article v-if="selectedPost && !loading" class="article-detail"><a class="back-link" href="/user">← 返回文章列表</a><p class="eyebrow">{{ new Date(selectedPost.updatedAt).toLocaleDateString('zh-CN') }}</p><h1>{{ selectedPost.title }}</h1><p v-if="selectedPost.excerpt" class="article-excerpt">{{ selectedPost.excerpt }}</p><div class="article-body">{{ selectedPost.content }}</div></article>
      <template v-else>
      <div class="content-heading"><div><h2>最新文章</h2><span>{{ total }} 篇</span></div><form class="search-bar" @submit.prevent="submitSearch"><input v-model="search" type="search" placeholder="搜索文章" aria-label="搜索文章" /><button class="text-button" type="submit">搜索</button></form></div>
      <div v-if="loading" class="empty-state">正在加载文章…</div>
      <div v-else-if="error" class="empty-state empty-error">{{ error }}</div>
      <div v-else-if="posts.length === 0" class="empty-state"><strong>还没有公开文章</strong><span>登录管理端发布第一篇内容吧。</span><a href="/admin" class="button button-dark">进入管理端</a></div>
      <template v-else><div class="post-list"><article v-for="post in posts" :key="post.id" class="post-card"><div class="post-meta"><span>文章</span><time>{{ new Date(post.updatedAt).toLocaleDateString('zh-CN') }}</time></div><h3>{{ post.title }}</h3><p>{{ post.excerpt || post.content.slice(0, 120) }}</p><a :href="`/user/${post.slug}`">继续阅读 <span>↗</span></a></article></div><div v-if="totalPages > 1" class="pagination"><button class="text-button" :disabled="page === 1" @click="changePage(page - 1)">上一页</button><span>{{ page }} / {{ totalPages }}</span><button class="text-button" :disabled="page === totalPages" @click="changePage(page + 1)">下一页</button></div></template>
      </template>
    </main>
    <footer class="public-footer"><span>Built for the next chapter.</span><span>v{{ status.version }}</span></footer>
  </div>
</template>
