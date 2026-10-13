<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { ApiError } from '../api'
import { listPosts, listTerms } from '../api/content'
import type { Post, Term } from '../types'

const props = withDefaults(defineProps<{ pageSize?: number; siteName?: string; siteDescription?: string }>(), { pageSize: 10, siteName: '', siteDescription: '' })
const pageSize = computed(() => (props.pageSize >= 1 && props.pageSize <= 50 ? props.pageSize : 10))

const posts = ref<Post[]>([])
const terms = ref<Term[]>([])
const termsPage = ref(0)
const termsTotal = ref(0)
const termsLoading = ref(false)
const termsError = ref(false)
const termsRetryPage = ref(1)
const termsPerBatch = 24
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
  const requestedPage = filters.get('page') ?? ''
  page.value = /^[1-9]\d*$/.test(requestedPage) && Number.isSafeInteger(Number(requestedPage)) ? Number(requestedPage) : 1
}

function updateFilterURL(replace = false) {
  const query = new URLSearchParams()
  if (categoryFilter.value) query.set('category', categoryFilter.value)
  if (tagFilter.value) query.set('tag', tagFilter.value)
  if (monthFilter.value) query.set('month', monthFilter.value)
  if (submittedSearch.value.trim()) query.set('q', submittedSearch.value.trim())
  if (page.value > 1) query.set('page', String(page.value))
  const suffix = query.toString() ? `?${query.toString()}` : ''
  const method = replace ? 'replaceState' : 'pushState'
  window.history[method]({}, '', `${window.location.pathname}${suffix}`)
}

async function loadPosts() {
  loading.value = true
  error.value = ''
  try {
    let response = await listPosts({
      search: submittedSearch.value,
      category: categoryFilter.value,
      tag: tagFilter.value,
      month: monthFilter.value,
      page: page.value,
      pageSize: pageSize.value,
    })
    const lastPage = Math.max(1, Math.ceil(response.total / pageSize.value))
    if (page.value > lastPage) {
      page.value = lastPage
      updateFilterURL(true)
      response = await listPosts({
        search: submittedSearch.value,
        category: categoryFilter.value,
        tag: tagFilter.value,
        month: monthFilter.value,
        page: page.value,
        pageSize: pageSize.value,
      })
    }
    posts.value = response.items
    total.value = response.total
  } catch (reason) {
    error.value = reason instanceof ApiError ? reason.message : '文章加载失败'
  } finally {
    loading.value = false
  }
}

async function loadTerms(pageNo = 1) {
  if (termsLoading.value) return
  termsRetryPage.value = pageNo
  termsLoading.value = true
  termsError.value = false
  try {
    const response = await listTerms(false, pageNo, termsPerBatch)
    terms.value = pageNo === 1 ? response.items : [...terms.value, ...response.items]
    termsPage.value = response.page
    termsTotal.value = response.total
  } catch {
    if (pageNo === 1) terms.value = []
    termsError.value = true
  } finally {
    termsLoading.value = false
  }
}

function loadMoreTerms() {
  if (!termsLoading.value && terms.value.length < termsTotal.value) {
    void loadTerms(termsPage.value + 1)
  }
}

function publishedParts(post: Post) {
  const raw = post.publishedAt || post.updatedAt
  const date = new Date(raw)
  if (Number.isNaN(date.getTime())) return { iso: raw, day: '--', rest: '' }
  return {
    iso: raw,
    day: String(date.getDate()).padStart(2, '0'),
    rest: `${date.getFullYear()}.${String(date.getMonth() + 1).padStart(2, '0')}`,
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

function changePage(nextPage: number) {
  if (nextPage < 1 || nextPage > totalPages.value || nextPage === page.value) return
  page.value = nextPage
  updateFilterURL()
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
const categories = computed(() => terms.value.filter((term) => term.kind === 'category'))
const tags = computed(() => terms.value.filter((term) => term.kind === 'tag'))
const heading = computed(() => {
  if (monthFilter.value) return `${monthFilter.value} 的文章`
  if (submittedSearch.value) return `搜索“${submittedSearch.value}”`
  if (categoryFilter.value || tagFilter.value) return '筛选结果'
  return '最新文章'
})
const listEmpty = computed(() => Boolean(submittedSearch.value || categoryFilter.value || tagFilter.value || monthFilter.value))
const activeFilterLabel = computed(() => {
  const parts: string[] = []
  if (submittedSearch.value) parts.push(`“${submittedSearch.value}”`)
  const slug = categoryFilter.value || tagFilter.value
  if (slug) parts.push(terms.value.find((term) => term.slug === slug)?.name || slug)
  if (monthFilter.value) parts.push(monthFilter.value)
  return parts.join(' · ')
})

function clearSearch() {
  search.value = ''
  submittedSearch.value = ''
  page.value = 1
  updateFilterURL()
  void loadPosts()
}

function clearAllFilters() {
  search.value = ''
  submittedSearch.value = ''
  categoryFilter.value = ''
  tagFilter.value = ''
  monthFilter.value = ''
  page.value = 1
  updateFilterURL()
  void loadPosts()
}

function blurSearch(event: KeyboardEvent) {
  if (event.key !== 'Escape') return
  const target = event.target
  if (target instanceof HTMLInputElement) target.blur()
}

function onKeydown(event: KeyboardEvent) {
  const target = event.target
  const typing = target instanceof HTMLElement && (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable)
  if (event.key === '/' && !typing && !event.metaKey && !event.ctrlKey && !event.altKey) {
    event.preventDefault()
    document.getElementById('reader-search')?.focus()
  }
}

onMounted(async () => {
  syncFiltersFromURL()
  window.addEventListener('popstate', handlePopState)
  window.addEventListener('keydown', onKeydown)
  await Promise.all([loadPosts(), loadTerms()])
})

onUnmounted(() => {
  window.removeEventListener('popstate', handlePopState)
  window.removeEventListener('keydown', onKeydown)
})
</script>

<template>
  <header class="list-mast">
    <p class="eyebrow">文章</p>
    <h1>{{ siteName || '博客' }}</h1>
    <p v-if="siteDescription">{{ siteDescription }}</p>
    <form class="search-bar" @submit.prevent="submitSearch">
      <input id="reader-search" v-model="search" type="search" maxlength="200" placeholder="搜索标题或正文" aria-label="搜索文章" aria-describedby="reader-search-hint" @keydown="blurSearch" />
      <button class="button button-primary" type="submit">搜索</button>
      <button v-if="submittedSearch" class="button" type="button" @click="clearSearch">清除</button>
    </form>
    <p id="reader-search-hint" class="search-hint">最多 200 个字符；按 / 聚焦搜索，Esc 取消焦点</p>
  </header>
  <div v-if="terms.length || termsError" class="term-filter-bar" aria-label="按分类或标签筛选">
    <button class="term-filter" type="button" :class="{ active: !categoryFilter && !tagFilter }" @click="clearTerms">全部</button>
    <details v-if="categories.length" class="term-group">
      <summary>分类 <small>{{ categories.length }}</small></summary>
      <div class="term-group-options">
        <a v-for="term in categories" :key="term.id" class="term-filter" :class="{ active: categoryFilter === term.slug }" :href="termURL(term)" @click.prevent="selectTerm(term)">{{ term.name }} <small>{{ term.postCount }}</small></a>
      </div>
    </details>
    <details v-if="tags.length" class="term-group">
      <summary>标签 <small>{{ tags.length }}</small></summary>
      <div class="term-group-options">
        <a v-for="term in tags" :key="term.id" class="term-filter" :class="{ active: tagFilter === term.slug }" :href="termURL(term)" @click.prevent="selectTerm(term)">{{ term.name }} <small>{{ term.postCount }}</small></a>
      </div>
    </details>
    <div v-if="termsError || terms.length < termsTotal" class="term-load-control">
      <span>{{ termsError ? '分类和标签加载失败' : `已载入 ${terms.length} / ${termsTotal} 个筛选` }}</span>
      <button class="text-button" type="button" :disabled="termsLoading" @click="termsError ? loadTerms(termsRetryPage) : loadMoreTerms()">{{ termsLoading ? '加载中…' : termsError ? '重试' : '加载更多' }}</button>
    </div>
  </div>
  <div v-if="activeFilterLabel" class="month-filter-bar"><span>当前筛选 {{ activeFilterLabel }}</span><button class="text-button" type="button" @click="clearAllFilters">查看全部</button></div>
  <div class="content-heading">
    <div><h2>{{ heading }}</h2><span>共 {{ total }} 篇</span></div>
  </div>

  <div v-if="loading" class="empty-state">正在加载文章…</div>
  <div v-else-if="error" class="empty-state empty-error">{{ error }}</div>
  <div v-else-if="posts.length === 0" class="empty-state">
    <strong>{{ listEmpty ? '没有匹配的文章' : '还没有公开文章' }}</strong>
    <span>{{ listEmpty ? '换一个词，或清掉当前筛选。' : '登录管理端，写下第一篇吧。' }}</span>
    <button v-if="listEmpty" class="button button-dark" type="button" @click="clearAllFilters">清除筛选</button>
    <a v-else href="/admin" class="button button-dark">进入管理端</a>
  </div>
  <template v-else>
    <div class="post-list">
      <article v-for="post in posts" :key="post.id" class="post-card">
        <time :datetime="publishedParts(post).iso"><span class="post-day">{{ publishedParts(post).day }}</span><span class="post-month">{{ publishedParts(post).rest }}</span></time>
        <div class="post-card-main">
          <h3><a :href="`/user/${post.slug}`">{{ post.title }}</a></h3>
          <div v-if="post.categories?.length || post.tags?.length" class="post-terms"><a v-for="term in [...(post.categories ?? []), ...(post.tags ?? [])]" :key="term.id" :href="termURL(term)">{{ term.name }}</a></div>
          <p v-if="post.excerpt">{{ post.excerpt }}</p>
        </div>
        <a class="post-open" :href="`/user/${post.slug}`">阅读</a>
      </article>
    </div>
    <div v-if="totalPages > 1" class="pagination"><button class="text-button" type="button" :disabled="page === 1" @click="changePage(page - 1)">上一页</button><span>{{ page }} / {{ totalPages }}</span><button class="text-button" type="button" :disabled="page === totalPages" @click="changePage(page + 1)">下一页</button></div>
  </template>
</template>
