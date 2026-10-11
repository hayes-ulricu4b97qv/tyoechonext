<script setup lang="ts">
import DOMPurify from 'dompurify'
import { marked } from 'marked'
import { computed, onMounted, reactive, ref } from 'vue'
import { ApiError } from '../api'
import { createComment, getPost, listPostComments } from '../api/content'
import type { Comment, Post, Status, Term } from '../types'

const props = defineProps<{ status: Status }>()
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
const replyTo = ref<Comment | null>(null)
const commentForm = reactive({ authorName: '', authorEmail: '', authorUrl: '', content: '' })
const commentsOpen = computed(() => selectedPost.value?.allowComment !== false && props.status.reading?.commentsEnabled !== false)
const repliesOpen = computed(() => commentsOpen.value && props.status.reading?.commentsThreaded !== false)
const threadedComments = computed(() => threadComments(comments.value))
const renderedContent = computed(() => {
  const html = marked.parse(selectedPost.value?.content ?? '', { async: false })
  return DOMPurify.sanitize(String(html))
})

function termURL(term: Term) {
  return `/user?${term.kind}=${encodeURIComponent(term.slug)}`
}

function safeURL(value?: string) {
  return !!value && /^https?:\/\//i.test(value)
}

function threadComments(items: Comment[]) {
  const children = new Map<string, Comment[]>()
  const roots: Comment[] = []
  const known = new Set(items.map((comment) => comment.id))
  for (const comment of items) {
    if (comment.parentId && known.has(comment.parentId)) {
      const list = children.get(comment.parentId) ?? []
      list.push(comment)
      children.set(comment.parentId, list)
    } else {
      roots.push(comment)
    }
  }
  const byTime = (left: Comment, right: Comment) => left.createdAt.localeCompare(right.createdAt)
  roots.sort(byTime)
  const flat: Array<Comment & { depth: number }> = []
  const walk = (comment: Comment, depth: number) => {
    flat.push({ ...comment, depth })
    for (const child of (children.get(comment.id) ?? []).sort(byTime)) walk(child, Math.min(depth + 1, 2))
  }
  for (const root of roots) walk(root, 0)
  return flat
}

function startReply(comment: Comment) {
  replyTo.value = comment
  commentMessage.value = ''
  commentError.value = ''
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
    const created = await createComment(postSlug, { ...commentForm, parentId: replyTo.value?.id ?? '' })
    commentForm.content = ''
    replyTo.value = null
    commentMessage.value = created.status === 'approved' ? '评论已发布。' : '评论已提交，审核通过后会显示。'
    if (created.status === 'approved') await loadComments()
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
    <nav v-if="selectedPost.previous || selectedPost.next" class="post-neighbors" aria-label="相邻文章">
      <a v-if="selectedPost.previous" :href="`/user/${selectedPost.previous.slug}`">上一篇：{{ selectedPost.previous.title }}</a>
      <span v-else></span>
      <a v-if="selectedPost.next" :href="`/user/${selectedPost.next.slug}`">下一篇：{{ selectedPost.next.title }}</a>
    </nav>

    <section class="comment-section">
      <div class="comment-heading"><div><p class="eyebrow">CONVERSATION</p><h2>评论</h2></div><span>{{ comments.length }} 条</span></div>
      <div v-if="commentsLoading" class="comment-state">正在加载评论…</div>
      <div v-else-if="threadedComments.length === 0" class="comment-state">{{ commentsOpen ? '还没有评论，来留下第一条吧。' : '评论已关闭。' }}</div>
      <div v-else class="comment-list">
        <article v-for="comment in threadedComments" :key="comment.id" class="comment-card" :data-depth="comment.depth">
          <div class="comment-meta">
            <a v-if="safeURL(comment.authorUrl)" :href="comment.authorUrl" rel="nofollow noreferrer" target="_blank">{{ comment.authorName }}</a>
            <strong v-else>{{ comment.authorName }}</strong>
            <time>{{ new Date(comment.createdAt).toLocaleDateString('zh-CN') }}</time>
          </div>
          <p>{{ comment.content }}</p>
          <button v-if="repliesOpen && comment.depth < 2" class="text-button" type="button" @click="startReply(comment)">回复</button>
        </article>
      </div>
      <p v-if="!commentsOpen && threadedComments.length > 0" class="comment-state">评论已关闭。</p>
      <form v-if="commentsOpen" class="comment-form" @submit.prevent="submitComment">
        <div v-if="replyTo" class="comment-reply-target"><span>回复 {{ replyTo.authorName }}</span><button class="text-button" type="button" @click="replyTo = null">取消</button></div>
        <div class="form-grid"><label>称呼<input v-model="commentForm.authorName" required maxlength="80" placeholder="你的名字" /></label><label>邮箱（可选）<input v-model="commentForm.authorEmail" type="email" placeholder="you@example.com" /></label></div>
        <label>网站（可选）<input v-model="commentForm.authorUrl" type="url" maxlength="200" placeholder="https://example.com" /></label>
        <label>评论<textarea v-model="commentForm.content" required maxlength="4000" rows="4" placeholder="写下你的想法…"></textarea></label>
        <p v-if="commentError" class="form-error">{{ commentError }}</p>
        <p v-if="commentMessage" class="form-success">{{ commentMessage }}</p>
        <button class="button button-dark" :disabled="commentSubmitting">{{ commentSubmitting ? '提交中…' : replyTo ? '发表回复' : '发表评论' }}</button>
      </form>
    </section>
  </article>
</template>
