<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ApiError } from '../api'
import { listOutboundMail } from '../api/auth'
import type { OutboundMail } from '../types'

const pageSize = 20
const page = ref(1)
const total = ref(0)
const items = ref<OutboundMail[]>([])
const error = ref('')
const pageCount = computed(() => Math.max(1, Math.ceil(total.value / pageSize)))

async function load() {
  error.value = ''
  try {
    const result = await listOutboundMail(page.value, pageSize)
    items.value = result.items
    total.value = result.total
  } catch (reason) {
    error.value = reason instanceof ApiError ? reason.message : '站内通知加载失败'
  }
}

async function changePage(next: number) {
  if (next < 1 || next > pageCount.value) return
  page.value = next
  await load()
}

onMounted(load)
</script>

<template>
  <section class="admin-section" id="mail">
    <div class="section-heading">
      <div><p class="eyebrow">通知</p><h2>站内通知</h2></div>
      <span class="section-count">{{ total }} 条</span>
    </div>
    <p class="admin-lead">待审核评论的通知会保存在这里；配置 SMTP 后也会发送到站点邮箱。</p>
    <p v-if="error" class="form-error">{{ error }}</p>
    <div v-if="items.length === 0" class="empty-state">还没有站内通知。</div>
    <div v-else class="admin-comments">
      <article v-for="message in items" :key="message.id" class="admin-mail-row">
        <div class="comment-row-content">
          <div class="comment-row-meta">
            <strong>{{ message.subject }}</strong>
            <span>{{ message.recipient }}</span>
            <time>{{ new Date(message.createdAt).toLocaleString('zh-CN') }}</time>
          </div>
          <p class="mail-body">{{ message.body }}</p>
        </div>
      </article>
    </div>
    <div v-if="total > pageSize" class="pagination-controls">
      <button class="button button-small" type="button" :disabled="page <= 1" @click="changePage(page - 1)">上一页</button>
      <span>第 {{ page }} / {{ pageCount }} 页</span>
      <button class="button button-small" type="button" :disabled="page >= pageCount" @click="changePage(page + 1)">下一页</button>
    </div>
  </section>
</template>
