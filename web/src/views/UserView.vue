<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { listArchive, listPages, listPosts, listRecentComments } from '../api/content'
import PostDetailView from './PostDetailView.vue'
import PostListView from './PostListView.vue'
import PageDetailView from './PageDetailView.vue'
import type { Comment, MonthCount, Page, Post, Status } from '../types'

defineProps<{ status: Status }>()

const postSlug = window.location.pathname.match(/^\/user\/(.+)$/)?.[1] ?? ''
const pageSlug = window.location.pathname.match(/^\/page\/(.+)$/)?.[1] ?? ''
const pages = ref<Page[]>([])
const archive = ref<MonthCount[]>([])
const recentPosts = ref<Post[]>([])
const recentComments = ref<Comment[]>([])

function monthLabel(month: string) {
  const [year, value] = month.split('-')
  return `${year}年${Number(value)}月`
}

onMounted(async () => {
  try {
    const response = await listPages(false, 1, 50)
    pages.value = response.items
  } catch {
    pages.value = []
  }
  try {
    archive.value = (await listArchive()).items
  } catch {
    archive.value = []
  }
  try {
    recentPosts.value = (await listPosts({ order: 'published', page: 1, pageSize: 6 })).items
  } catch {
    recentPosts.value = []
  }
  try {
    recentComments.value = (await listRecentComments(1, 6)).items
  } catch {
    recentComments.value = []
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
      <p v-if="status.site?.description" class="site-description">{{ status.site.description }}</p>
      <div class="reader-layout">
        <div class="reader-main">
          <PageDetailView v-if="pageSlug" />
          <PostDetailView v-else-if="postSlug" :status="status" />
          <PostListView v-else :page-size="status.reading?.postsPerPage || 10" />
        </div>
        <aside class="reader-rail" aria-label="归档与最新动态">
          <section aria-label="月份归档">
            <h2>归档</h2>
            <a v-for="item in archive" :key="item.month" :href="`/user?month=${item.month}`">{{ monthLabel(item.month) }} <small>{{ item.count }}</small></a>
            <p v-if="archive.length === 0" class="rail-empty">还没有已发布文章</p>
          </section>
          <section aria-label="最新文章">
            <h2>最新文章</h2>
            <a v-for="post in recentPosts" :key="post.id" :href="`/user/${post.slug}`">{{ post.title }}</a>
            <p v-if="recentPosts.length === 0" class="rail-empty">还没有已发布文章</p>
          </section>
          <section aria-label="最近回复">
            <h2>最近回复</h2>
            <a v-for="comment in recentComments" :key="comment.id" :href="`/user/${comment.postSlug}`">{{ comment.authorName }}：{{ comment.content }}</a>
            <p v-if="recentComments.length === 0" class="rail-empty">还没有已通过的评论</p>
          </section>
        </aside>
      </div>
    </main>
    <footer class="public-footer"><span>Built for the next chapter.</span><span>v{{ status.version }}</span></footer>
  </div>
</template>
