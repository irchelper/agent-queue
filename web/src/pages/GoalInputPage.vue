<script setup lang="ts">
import { ref, computed } from 'vue'
import { useRouter } from 'vue-router'
import AppLayout from '@/layouts/AppLayout.vue'

interface TemplateTask {
  assigned_to: string
  title: string
  description?: string
}

interface Template {
  id: number
  name: string
  description: string
  tasks: TemplateTask[]
}

const router = useRouter()
const goalText = ref('')
const submitting = ref(false)
const error = ref<string | null>(null)

// Template matching state
const templates = ref<Template[]>([])
const loadingTemplates = ref(false)
const selectedTemplate = ref<Template | null>(null)
const previewMode = ref(false)

// Preview: tasks with variables substituted
const previewTasks = computed<TemplateTask[]>(() => {
  if (!selectedTemplate.value) return []
  const goal = goalText.value.trim()
  return selectedTemplate.value.tasks.map(t => ({
    assigned_to: t.assigned_to,
    title: t.title.replace(/\{goal\}/g, goal).replace(/\{[^}]+\}/g, goal),
    description: (t.description ?? '').replace(/\{goal\}/g, goal).replace(/\{[^}]+\}/g, goal),
  }))
})

const agentColors: Record<string, string> = {
  thinker: 'bg-purple-500/20 text-purple-400 border-purple-500/30',
  coder: 'bg-blue-500/20 text-blue-400 border-blue-500/30',
  qa: 'bg-green-500/20 text-green-400 border-green-500/30',
  devops: 'bg-cyan-500/20 text-cyan-400 border-cyan-500/30',
  writer: 'bg-yellow-500/20 text-yellow-400 border-yellow-500/30',
  pm: 'bg-pink-500/20 text-pink-400 border-pink-500/30',
  security: 'bg-red-500/20 text-red-400 border-red-500/30',
  human: 'bg-amber-500/20 text-amber-400 border-amber-500/30',
}
function agentColor(name: string) {
  return agentColors[name] ?? 'bg-gray-700/60 text-gray-400 border-gray-600/30'
}

async function loadTemplates() {
  loadingTemplates.value = true
  try {
    const resp = await fetch('/templates')
    if (resp.ok) {
      const data = await resp.json()
      templates.value = data.templates ?? []
    }
  } catch {
    // silently ignore
  } finally {
    loadingTemplates.value = false
  }
}

// Load templates on mount
loadTemplates()

function selectTemplate(tpl: Template) {
  selectedTemplate.value = tpl
  previewMode.value = true
}

function clearTemplate() {
  selectedTemplate.value = null
  previewMode.value = false
}

async function submit() {
  const text = goalText.value.trim()
  if (!text) return
  submitting.value = true
  error.value = null
  try {
    let chainId: string | undefined

    if (selectedTemplate.value && previewTasks.value.length > 0) {
      // Use template: POST /dispatch/from-template/:name
      const resp = await fetch(`/dispatch/from-template/${encodeURIComponent(selectedTemplate.value.name)}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          vars: { goal: text },
          notify_ceo_on_complete: true,
        }),
      })
      if (!resp.ok) throw new Error(`HTTP ${resp.status}`)
      const data = await resp.json()
      chainId = data.chain_id || data.tasks?.[0]?.chain_id
    } else {
      // Plain dispatch to thinker
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
      chainId = data.chain_id || data.tasks?.[0]?.chain_id
    }

    router.push('/goals')
    void chainId
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
        <p class="text-gray-500 text-sm">描述你的目标，选择模板拆解成任务链路，或直接提交给 thinker 分析。</p>
      </div>

      <!-- Input area -->
      <div class="bg-gray-900 border border-gray-700 rounded-2xl p-5 mb-5 focus-within:border-blue-500/50 transition-colors">
        <textarea
          v-model="goalText"
          class="w-full bg-transparent text-gray-100 text-base resize-none placeholder-gray-600 leading-relaxed focus:outline-none"
          rows="4"
          placeholder="例如：帮我重构登录模块，要求代码质量达标、通过安全审查并更新技术文档..."
          :disabled="submitting"
          @keydown.ctrl.enter="submit"
          @keydown.meta.enter="submit"
        />
      </div>

      <!-- Template selector -->
      <div v-if="!previewMode" class="mb-5">
        <div class="flex items-center justify-between mb-3">
          <span class="text-xs font-semibold text-gray-500 uppercase tracking-wider">选择模板（可选）</span>
          <span v-if="loadingTemplates" class="text-xs text-gray-600">加载中…</span>
        </div>
        <div class="grid grid-cols-1 gap-2">
          <button
            v-for="tpl in templates"
            :key="tpl.id"
            class="w-full text-left bg-gray-900 border border-gray-700 hover:border-blue-500/50 rounded-xl p-3.5 transition-colors group"
            :disabled="!goalText.trim()"
            :class="!goalText.trim() ? 'opacity-40 cursor-not-allowed' : ''"
            @click="selectTemplate(tpl)"
          >
            <div class="flex items-start justify-between gap-3">
              <div class="flex-1 min-w-0">
                <div class="text-sm font-medium text-gray-200 group-hover:text-blue-400 transition-colors">{{ tpl.name }}</div>
                <div class="text-xs text-gray-500 mt-0.5">{{ tpl.description }}</div>
              </div>
              <span class="flex-shrink-0 text-xs text-gray-600 bg-gray-800 px-2 py-0.5 rounded-full mt-0.5">
                {{ tpl.tasks.length }} 步
              </span>
            </div>
          </button>

          <!-- No template: direct submit -->
          <button
            class="w-full text-left bg-gray-900/50 border border-dashed border-gray-700 hover:border-gray-500 rounded-xl p-3.5 transition-colors"
            :disabled="!goalText.trim() || submitting"
            :class="!goalText.trim() ? 'opacity-40 cursor-not-allowed' : ''"
            @click="submit"
          >
            <div class="text-sm text-gray-400">⚡ 直接提交给 thinker</div>
            <div class="text-xs text-gray-600 mt-0.5">由 thinker 自动分析拆解</div>
          </button>
        </div>
      </div>

      <!-- Preview mode -->
      <div v-if="previewMode && selectedTemplate" class="mb-5">
        <div class="flex items-center justify-between mb-3">
          <div>
            <span class="text-xs font-semibold text-gray-400">拆解预览</span>
            <span class="ml-2 text-xs text-gray-600">{{ selectedTemplate.name }}</span>
          </div>
          <button
            class="text-xs text-gray-500 hover:text-gray-300 transition-colors"
            @click="clearTemplate"
          >← 更换模板</button>
        </div>

        <!-- Task list preview -->
        <div class="space-y-2 mb-4">
          <div
            v-for="(task, idx) in previewTasks"
            :key="idx"
            class="bg-gray-900 border border-gray-700/60 rounded-xl p-3.5"
          >
            <div class="flex items-center gap-2 mb-1.5">
              <span class="text-xs text-gray-600 font-mono">{{ idx + 1 }}</span>
              <span
                class="text-xs px-2 py-0.5 rounded-md border font-medium"
                :class="agentColor(task.assigned_to)"
              >{{ task.assigned_to }}</span>
            </div>
            <div class="text-sm text-gray-200 font-medium">{{ task.title }}</div>
            <div v-if="task.description" class="text-xs text-gray-500 mt-1">{{ task.description }}</div>
          </div>
        </div>

        <!-- Confirm buttons -->
        <div class="flex gap-3">
          <button
            class="flex-1 bg-blue-600 hover:bg-blue-500 disabled:opacity-50 disabled:cursor-not-allowed text-white text-sm font-semibold px-5 py-2.5 rounded-xl transition-colors flex items-center justify-center gap-2"
            :disabled="!goalText.trim() || submitting"
            @click="submit"
          >
            <span v-if="submitting">⏳ 创建中…</span>
            <span v-else>✅ 确认创建 {{ previewTasks.length }} 个任务</span>
          </button>
          <button
            class="px-4 py-2.5 bg-gray-800 hover:bg-gray-700 text-gray-400 text-sm rounded-xl transition-colors"
            :disabled="submitting"
            @click="clearTemplate"
          >取消</button>
        </div>
      </div>

      <div
        v-if="error"
        class="p-3 bg-red-900/40 border border-red-500 rounded-lg text-red-300 text-sm mb-4"
      >{{ error }}</div>

      <!-- Tips -->
      <div class="bg-gray-900/50 border border-gray-800 rounded-xl p-4 text-xs text-gray-500 space-y-2">
        <div class="font-medium text-gray-400 mb-3">💡 提示</div>
        <div>• 先填写目标，再选择模板或直接提交</div>
        <div>• 选择模板可预览任务拆解结果后再确认</div>
        <div>• 提交后可在「目标追踪」查看链路进度</div>
      </div>
    </div>
  </AppLayout>
</template>
