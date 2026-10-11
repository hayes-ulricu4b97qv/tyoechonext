<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { listPages } from '../api/content'
import PostDetailView from './PostDetailView.vue'
import PostListView from './PostListView.vue'
import PageDetailView from './PageDetailView.vue'
import type { Page, PagePage, Status } from '../types'

defineProps<{ status: Status }>()

const postSlug = window.location.pathname.match(/^\/user\/(.+)$/)?.[1] ?? ''
const pageSlug = window.location.pathname.match(/^\/page\/(.+)$/)?.[1] ?? ''
const pages = ref<Page[]>([])

onMounted(async () => {
  try {
    const response = await listPages(false, 1, 50)
    pages.value = response.items
  } catch {
    pages.value = []
  }
})
</script>

<template>
  <div class="public-shell">
    <header class="public-nav">
      <a class="brand" href="/user"><span class="brand-mark">T</span> {{ status.site?.name ?? 'Typecho Next' }}</a>
      <nav><a href="/user" class="active">阅读</a><a v-for="page in pages" :key="page.id" :href="`/page/${page.slug}`">{{ page.title }}</a><a href="/admin">管理</a></nav>
    </header>
    <main class="public-content">
      <PageDetailView v-if="pageSlug" />
      <PostDetailView v-else-if="postSlug" />
      <PostListView v-else />
    </main>
    <footer class="public-footer"><span>Built for the next chapter.</span><span>v{{ status.version }}</span></footer>
  </div>
</template>
