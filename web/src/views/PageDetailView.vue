<script setup lang="ts">
import DOMPurify from 'dompurify'
import { marked } from 'marked'
import { computed, onMounted, ref } from 'vue'
import { ApiError } from '../api'
import { getPage } from '../api/content'
import type { Page } from '../types'

const rawSlug = window.location.pathname.match(/^\/page\/(.+)$/)?.[1] ?? ''
const pageSlug = rawSlug ? decodeURIComponent(rawSlug) : ''
const page = ref<Page | null>(null)
const loading = ref(true)
const error = ref('')
const renderedContent = computed(() => DOMPurify.sanitize(String(marked.parse(page.value?.content ?? '', { async: false }))))

onMounted(async () => {
  if (!pageSlug) {
    error.value = '页面地址无效'
    loading.value = false
    return
  }
  try {
    page.value = await getPage(pageSlug)
  } catch (reason) {
    error.value = reason instanceof ApiError ? reason.message : '页面加载失败'
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <div v-if="loading" class="empty-state">正在加载页面…</div>
  <div v-else-if="error" class="empty-state empty-error"><strong>{{ error }}</strong><a href="/user" class="button button-dark">返回文章列表</a></div>
  <article v-else-if="page" class="article-detail page-detail">
    <a class="back-link" href="/user">← 返回文章列表</a>
    <p class="eyebrow">PAGE · {{ new Date(page.updatedAt).toLocaleDateString('zh-CN') }}</p>
    <h1>{{ page.title }}</h1>
    <p v-if="page.excerpt" class="article-excerpt">{{ page.excerpt }}</p>
    <div class="article-body article-rich" v-html="renderedContent"></div>
  </article>
</template>
