<script setup lang="ts">
import DOMPurify from 'dompurify'
import { marked } from 'marked'
import { computed, onMounted, reactive, ref } from 'vue'
import { ApiError } from '../api'
import { createComment, getPost, listPostComments } from '../api/content'
import type { Comment, CommentPage, Post, Term } from '../types'

const rawSlug = window.location.pathname.match(/^\/user\/(.+)$/)?.[1] ?? ''
const postSlug = rawSlug ? decodeURIComponent(rawSlug) : ''
const selectedPost = ref<Post | null>(null)
const loading = ref(true)
const error = ref('')
const comments = ref<Comment[]>([])
const commentsLoading = ref(false)
const commentSubmitting = ref(false)
const commentError = ref('')
const commentMessage = ref('')
const commentForm = reactive({ authorName: '', authorEmail: '', content: '' })
const renderedContent = computed(() => {
  const html = marked.parse(selectedPost.value?.content ?? '', { async: false })
  return DOMPurify.sanitize(String(html))
})

function termURL(term: Term) {
  return `/user?${term.kind}=${encodeURIComponent(term.slug)}`
}

async function loadComments() {
  if (!postSlug) return
  commentsLoading.value = true
  commentError.value = ''
  try {
    const response = await listPostComments(postSlug, 1, 100)
    comments.value = response.items
  } catch (reason) {
    commentError.value = reason instanceof ApiError ? reason.message : '评论加载失败'
  } finally {
    commentsLoading.value = false
  }
}

async function submitComment() {
  if (!postSlug) return
  commentSubmitting.value = true
  commentError.value = ''
  commentMessage.value = ''
  try {
    await createComment(postSlug, commentForm)
    commentForm.authorName = ''
    commentForm.authorEmail = ''
    commentForm.content = ''
    commentMessage.value = '评论已提交，审核通过后会显示。'
  } catch (reason) {
    commentError.value = reason instanceof ApiError ? reason.message : '评论提交失败'
  } finally {
    commentSubmitting.value = false
  }
}

onMounted(async () => {
  if (!postSlug) {
    loading.value = false
    error.value = '文章地址无效'
    return
  }
  try {
    selectedPost.value = await getPost(postSlug)
    await loadComments()
  } catch (reason) {
    error.value = reason instanceof ApiError ? reason.message : '文章加载失败'
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <div v-if="loading" class="empty-state">正在加载文章…</div>
  <div v-else-if="error" class="empty-state empty-error"><strong>{{ error }}</strong><a href="/user" class="button button-dark">返回文章列表</a></div>
  <article v-else-if="selectedPost" class="article-detail">
    <a class="back-link" href="/user">← 返回文章列表</a>
    <p class="eyebrow">{{ new Date(selectedPost.updatedAt).toLocaleDateString('zh-CN') }}</p>
    <h1>{{ selectedPost.title }}</h1>
    <div v-if="selectedPost.categories?.length || selectedPost.tags?.length" class="post-terms"><a v-for="term in [...(selectedPost.categories ?? []), ...(selectedPost.tags ?? [])]" :key="term.id" :href="termURL(term)">{{ term.name }}</a></div>
    <p v-if="selectedPost.excerpt" class="article-excerpt">{{ selectedPost.excerpt }}</p>
    <div class="article-body article-rich" v-html="renderedContent"></div>

    <section class="comment-section">
      <div class="comment-heading"><div><p class="eyebrow">CONVERSATION</p><h2>评论</h2></div><span>{{ comments.length }} 条</span></div>
      <div v-if="commentsLoading" class="comment-state">正在加载评论…</div>
      <div v-else-if="comments.length === 0" class="comment-state">还没有评论，来留下第一条吧。</div>
      <div v-else class="comment-list"><article v-for="comment in comments" :key="comment.id" class="comment-card"><div class="comment-meta"><strong>{{ comment.authorName }}</strong><time>{{ new Date(comment.createdAt).toLocaleDateString('zh-CN') }}</time></div><p>{{ comment.content }}</p></article></div>
      <form class="comment-form" @submit.prevent="submitComment">
        <div class="form-grid"><label>称呼<input v-model="commentForm.authorName" required maxlength="80" placeholder="你的名字" /></label><label>邮箱（可选）<input v-model="commentForm.authorEmail" type="email" placeholder="you@example.com" /></label></div>
        <label>评论<textarea v-model="commentForm.content" required maxlength="4000" rows="4" placeholder="写下你的想法…"></textarea></label>
        <p v-if="commentError" class="form-error">{{ commentError }}</p>
        <p v-if="commentMessage" class="form-success">{{ commentMessage }}</p>
        <button class="button button-dark" :disabled="commentSubmitting">{{ commentSubmitting ? '提交中…' : '发表评论' }}</button>
      </form>
    </section>
  </article>
</template>
