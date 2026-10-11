<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { ApiError } from '../api'
import { listPosts, listTerms } from '../api/content'
import type { Post, PostPage, Term, TermPage } from '../types'

const props = withDefaults(defineProps<{ pageSize?: number }>(), { pageSize: 10 })
const pageSize = computed(() => (props.pageSize >= 1 && props.pageSize <= 50 ? props.pageSize : 10))

const posts = ref<Post[]>([])
const terms = ref<Term[]>([])
const loading = ref(true)
const error = ref('')
const search = ref('')
const submittedSearch = ref('')
const categoryFilter = ref('')
const tagFilter = ref('')
const monthFilter = ref('')
const page = ref(1)
const total = ref(0)

function syncFiltersFromURL() {
  const filters = new URLSearchParams(window.location.search)
  categoryFilter.value = filters.get('category') ?? ''
  tagFilter.value = filters.get('tag') ?? ''
  monthFilter.value = filters.get('month') ?? ''
  submittedSearch.value = filters.get('q') ?? ''
  search.value = submittedSearch.value
}

function updateFilterURL() {
  const query = new URLSearchParams()
  if (categoryFilter.value) query.set('category', categoryFilter.value)
  if (tagFilter.value) query.set('tag', tagFilter.value)
  if (monthFilter.value) query.set('month', monthFilter.value)
  if (submittedSearch.value.trim()) query.set('q', submittedSearch.value.trim())
  const suffix = query.toString() ? `?${query.toString()}` : ''
  window.history.pushState({}, '', `${window.location.pathname}${suffix}`)
}

async function loadPosts() {
  loading.value = true
  error.value = ''
  try {
    const response = await listPosts({
      search: submittedSearch.value,
      category: categoryFilter.value,
      tag: tagFilter.value,
      month: monthFilter.value,
      page: page.value,
      pageSize: pageSize.value,
    })
    posts.value = response.items
    total.value = response.total
  } catch (reason) {
    error.value = reason instanceof ApiError ? reason.message : '文章加载失败'
  } finally {
    loading.value = false
  }
}

async function loadTerms() {
  try {
    const response = await listTerms(false, 1, 100)
    terms.value = response.items
  } catch {
    terms.value = []
  }
}

function submitSearch() {
  submittedSearch.value = search.value
  page.value = 1
  updateFilterURL()
  void loadPosts()
}

function selectTerm(term: Term) {
  categoryFilter.value = term.kind === 'category' ? term.slug : ''
  tagFilter.value = term.kind === 'tag' ? term.slug : ''
  page.value = 1
  updateFilterURL()
  void loadPosts()
}

function clearTerms() {
  categoryFilter.value = ''
  tagFilter.value = ''
  page.value = 1
  updateFilterURL()
  void loadPosts()
}

function clearMonth() {
  monthFilter.value = ''
  page.value = 1
  updateFilterURL()
  void loadPosts()
}

function changePage(nextPage: number) {
  if (nextPage < 1 || nextPage > totalPages.value || nextPage === page.value) return
  page.value = nextPage
  void loadPosts()
}

function termURL(term: Term) {
  return `/user?${term.kind}=${encodeURIComponent(term.slug)}`
}

function handlePopState() {
  syncFiltersFromURL()
  page.value = 1
  void loadPosts()
}

const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)))

onMounted(async () => {
  syncFiltersFromURL()
  window.addEventListener('popstate', handlePopState)
  await Promise.all([loadPosts(), loadTerms()])
})

onUnmounted(() => {
  window.removeEventListener('popstate', handlePopState)
})
</script>

<template>
  <section class="hero-block"><p class="eyebrow">A NEW PUBLISHING CORE</p><h1>写下值得被看见的东西。</h1><p>这是全新 Go + Vue 架构的用户端。轻量、清晰，也给未来留出空间。</p></section>
  <div v-if="terms.length" class="term-filter-bar" aria-label="按分类或标签筛选">
    <button class="term-filter" :class="{ active: !categoryFilter && !tagFilter }" @click="clearTerms">全部</button>
    <a v-for="term in terms" :key="term.id" class="term-filter" :class="{ active: categoryFilter === term.slug || tagFilter === term.slug }" :href="termURL(term)" @click.prevent="selectTerm(term)">{{ term.name }} <small>{{ term.postCount }}</small></a>
  </div>
  <div v-if="monthFilter" class="month-filter-bar"><span>归档 {{ monthFilter }}</span><button class="text-button" type="button" @click="clearMonth">查看全部</button></div>
  <div class="content-heading"><div><h2>{{ monthFilter ? `${monthFilter} 的文章` : '最新文章' }}</h2><span>{{ total }} 篇</span></div><form class="search-bar" @submit.prevent="submitSearch"><input v-model="search" type="search" placeholder="搜索文章" aria-label="搜索文章" /><button class="text-button" type="submit">搜索</button></form></div>
  <div v-if="loading" class="empty-state">正在加载文章…</div>
  <div v-else-if="error" class="empty-state empty-error">{{ error }}</div>
  <div v-else-if="posts.length === 0" class="empty-state"><strong>还没有公开文章</strong><span>登录管理端发布第一篇内容吧。</span><a href="/admin" class="button button-dark">进入管理端</a></div>
  <template v-else>
    <div class="post-list"><article v-for="post in posts" :key="post.id" class="post-card"><div class="post-meta"><span>文章</span><time>{{ new Date(post.updatedAt).toLocaleDateString('zh-CN') }}</time></div><h3>{{ post.title }}</h3><div v-if="post.categories?.length || post.tags?.length" class="post-terms"><a v-for="term in [...(post.categories ?? []), ...(post.tags ?? [])]" :key="term.id" :href="termURL(term)">{{ term.name }}</a></div><p>{{ post.excerpt || post.content.slice(0, 120) }}</p><a :href="`/user/${post.slug}`">继续阅读 <span>↗</span></a></article></div>
    <div v-if="totalPages > 1" class="pagination"><button class="text-button" :disabled="page === 1" @click="changePage(page - 1)">上一页</button><span>{{ page }} / {{ totalPages }}</span><button class="text-button" :disabled="page === totalPages" @click="changePage(page + 1)">下一页</button></div>
  </template>
</template>
