<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import AppLayout from '@/layouts/AppLayout.vue'

const router = useRouter()
const goalText = ref('')
const submitting = ref(false)
const error = ref<string | null>(null)

async function submit() {
  const text = goalText.value.trim()
  if (!text) return
  submitting.value = true
  error.value = null
  try {
    const resp = await fetch('/dispatch/chain', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        tasks: [
          {
            title: text,
            assigned_to: 'thinker',
            description: `用户目标：${text}`,
            requires_review: false,
          },
        ],
        notify_ceo_on_complete: true,
        chain_title: text,
      }),
    })
    if (!resp.ok) throw new Error(`HTTP ${resp.status}`)
    const data = await resp.json()
    const chainId = data.chain_id || data.tasks?.[0]?.chain_id
    if (chainId) {
      router.push('/goals')
    } else {
      router.push('/goals')
    }
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e)
    submitting.value = false
  }
}
</script>

<template>
  <AppLayout>
    <div class="max-w-2xl mx-auto px-6 py-10">
      <div class="mb-8">
        <h1 class="text-2xl font-bold text-gray-100 mb-2">🎯 输入目标</h1>
        <p class="text-gray-500 text-sm">描述你的目标，系统将自动拆解成任务链路并分配给对应 agent。</p>
      </div>

      <!-- Input area -->
      <div class="bg-gray-900 border border-gray-700 rounded-2xl p-5 mb-5 focus-within:border-blue-500/50 transition-colors">
        <textarea
          v-model="goalText"
          class="w-full bg-transparent text-gray-100 text-base resize-none placeholder-gray-600 leading-relaxed focus:outline-none"
          rows="5"
          placeholder="例如：帮我重构登录模块，要求代码质量达标、通过安全审查并更新技术文档..."
          :disabled="submitting"
          @keydown.ctrl.enter="submit"
          @keydown.meta.enter="submit"
        />
        <div class="flex items-center justify-between mt-4 pt-4 border-t border-gray-800">
          <span class="text-xs text-gray-600">⌘/Ctrl + Enter 提交</span>
          <button
            class="bg-blue-600 hover:bg-blue-500 disabled:opacity-50 disabled:cursor-not-allowed text-white text-sm font-semibold px-5 py-2 rounded-xl transition-colors flex items-center gap-2"
            :disabled="!goalText.trim() || submitting"
            @click="submit"
          >
            <span v-if="submitting">⏳ 创建中…</span>
            <span v-else>⚡ 创建任务链</span>
          </button>
        </div>
      </div>

      <div
        v-if="error"
        class="p-3 bg-red-900/40 border border-red-500 rounded-lg text-red-300 text-sm"
      >{{ error }}</div>

      <!-- Tips -->
      <div class="bg-gray-900/50 border border-gray-800 rounded-xl p-4 text-xs text-gray-500 space-y-2">
        <div class="font-medium text-gray-400 mb-3">💡 提示</div>
        <div>• 目标越具体，拆解越准确</div>
        <div>• 提交后可在「目标追踪」查看链路进度</div>
        <div>• 任务将分配给 thinker 进行架构分析</div>
      </div>
    </div>
  </AppLayout>
</template>
