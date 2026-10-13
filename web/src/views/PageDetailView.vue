<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { ApiError } from '../api'
import { getPage } from '../api/content'
import { renderMarkdown } from '../markdown'
import { setDocumentMeta } from '../meta'
import type { Page, Status } from '../types'

const props = defineProps<{ status: Status }>()

const rawSlug = window.location.pathname.match(/^\/page\/(.+)$/)?.[1] ?? ''
const pageSlug = rawSlug ? decodeURIComponent(rawSlug) : ''
const page = ref<Page | null>(null)
const loading = ref(true)
const error = ref('')
const progress = ref(0)
const renderedContent = computed(() => renderMarkdown(page.value?.content ?? ''))
const readingLabel = computed(() => {
  const text = page.value?.content.replace(/\s+/g, '') ?? ''
  if (!text) return ''
  return `约 ${Math.max(1, Math.ceil(text.length / 450))} 分钟`
})

function updateProgress() {
  const height = document.documentElement.scrollHeight - window.innerHeight
  progress.value = height <= 0 ? 0 : Math.min(100, Math.max(0, (window.scrollY / height) * 100))
}

onMounted(async () => {
  updateProgress()
  window.addEventListener('scroll', updateProgress, { passive: true })
  if (!pageSlug) {
    error.value = '页面地址无效'
    loading.value = false
    return
  }
  try {
    page.value = await getPage(pageSlug)
    const site = props.status.site
    const title = site?.name ? `${page.value.title} · ${site.name}` : page.value.title
    setDocumentMeta(title, page.value.excerpt || site?.description || '', site?.keywords ?? '')
  } catch (reason) {
    error.value = reason instanceof ApiError ? reason.message : '页面加载失败'
  } finally {
    loading.value = false
    updateProgress()
  }
})

onUnmounted(() => window.removeEventListener('scroll', updateProgress))
</script>

<template>
  <div v-if="loading" class="empty-state">正在加载页面…</div>
  <div v-else-if="error" class="empty-state empty-error"><strong>{{ error }}</strong><a href="/user" class="button button-dark">返回文章列表</a></div>
  <article v-else-if="page" class="article-detail page-detail">
    <div class="read-progress" aria-hidden="true"><span :style="{ width: progress + '%' }"></span></div>
    <header class="article-head">
      <a class="back-link" href="/user">全部文章</a>
      <p class="eyebrow">页面 · {{ new Date(page.updatedAt).toLocaleDateString('zh-CN') }}<template v-if="readingLabel"> · {{ readingLabel }}</template></p>
      <h1>{{ page.title }}</h1>
    </header>
    <p v-if="page.excerpt" class="article-excerpt">{{ page.excerpt }}</p>
    <div class="article-body article-rich" v-html="renderedContent"></div>
  </article>
</template>
