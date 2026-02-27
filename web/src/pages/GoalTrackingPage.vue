<script setup lang="ts">
import { ref } from 'vue'
import AppLayout from '@/layouts/AppLayout.vue'
import { usePolling } from '@/composables/usePolling'
import { api } from '@/api/client'
import type { Task } from '@/types'

interface ChainGroup {
  chain_id: string
  tasks: Task[]
}

const chains = ref<ChainGroup[]>([])
const expanded = ref<Set<string>>(new Set())

async function fetchChains() {
  const resp = await api.getChains()
  chains.value = resp.chains ?? []
}

const { loading, error } = usePolling(fetchChains, 30_000)

function doneCount(tasks: Task[]) {
  return tasks.filter((t) => t.status === 'done' || t.status === 'cancelled').length
}

function progressPct(tasks: Task[]) {
  if (!tasks.length) return 0
  return Math.round((doneCount(tasks) / tasks.length) * 100)
}

function isHuman(task: Task) {
  return task.assigned_to === 'human'
}

function taskIcon(task: Task) {
  return isHuman(task) ? '👤' : '🤖'
}

function statusColor(status: string) {
  const map: Record<string, string> = {
    done: 'text-green-400',
    in_progress: 'text-blue-400',
    pending: 'text-yellow-400',
    failed: 'text-red-400',
    blocked: 'text-orange-400',
    cancelled: 'text-gray-500',
    claimed: 'text-cyan-400',
    review: 'text-purple-400',
  }
  return map[status] ?? 'text-gray-400'
}

function statusDot(status: string) {
  const map: Record<string, string> = {
    done: 'bg-green-400',
    in_progress: 'bg-blue-400',
    pending: 'bg-yellow-400',
    failed: 'bg-red-400',
    blocked: 'bg-orange-400',
    cancelled: 'bg-gray-600',
    claimed: 'bg-cyan-400',
    review: 'bg-purple-400',
  }
  return map[status] ?? 'bg-gray-500'
}

function chainTitle(chain: ChainGroup) {
  // Best-effort: use first task title or chain_id suffix
  return chain.tasks[0]?.title ?? chain.chain_id.slice(-12)
}

function toggleExpand(chainId: string) {
  if (expanded.value.has(chainId)) {
    expanded.value.delete(chainId)
  } else {
    expanded.value.add(chainId)
  }
}

// Segment progress bar by agent vs human portions
function segmentsFor(tasks: Task[]) {
  return tasks.map((t) => ({
    done: t.status === 'done' || t.status === 'cancelled',
    human: isHuman(t),
    status: t.status,
  }))
}
</script>

<template>
  <AppLayout>
    <div class="p-6">
      <div class="flex items-center justify-between mb-6">
        <div>
          <h1 class="text-xl font-bold text-gray-100">📈 目标追踪</h1>
          <p class="text-gray-500 text-sm mt-1">任务链路进度概览</p>
        </div>
        <router-link
          to="/goals/new"
          class="bg-blue-600 hover:bg-blue-500 text-white text-sm font-medium px-4 py-2 rounded-xl transition-colors"
        >+ 新建目标</router-link>
      </div>

      <div v-if="error" class="mb-4 p-3 bg-red-900/40 border border-red-500 rounded text-red-300 text-sm">{{ error }}</div>
      <div v-if="loading && !chains.length" class="text-gray-600 text-center py-20">加载中…</div>
      <div v-else-if="!chains.length" class="text-center py-20 text-gray-600">
        <div class="text-4xl mb-3">🎯</div>
        <div class="text-sm mb-4">暂无链路任务</div>
        <router-link to="/goals/new" class="text-blue-400 text-sm hover:underline">创建第一个目标 →</router-link>
      </div>

      <div v-else class="space-y-4">
        <div
          v-for="chain in chains"
          :key="chain.chain_id"
          class="bg-gray-900 border border-gray-700/60 rounded-2xl overflow-hidden hover:border-gray-600 transition-colors"
        >
          <!-- Chain header -->
          <div
            class="px-5 py-4 cursor-pointer flex items-center gap-4"
            @click="toggleExpand(chain.chain_id)"
          >
            <div class="flex-1 min-w-0">
              <div class="flex items-center gap-2 mb-2">
                <h3 class="font-medium text-gray-100 text-sm truncate">{{ chainTitle(chain) }}</h3>
                <span class="text-xs text-gray-500 shrink-0">
                  {{ doneCount(chain.tasks) }}/{{ chain.tasks.length }}
                </span>
              </div>
              <!-- Progress bar segments -->
              <div class="flex gap-0.5 h-2 rounded-full overflow-hidden bg-gray-800">
                <div
                  v-for="(seg, i) in segmentsFor(chain.tasks)"
                  :key="i"
                  class="flex-1 rounded-sm transition-all"
                  :class="[
                    seg.done
                      ? (seg.human ? 'bg-orange-400' : 'bg-blue-500')
                      : seg.status === 'in_progress'
                        ? (seg.human ? 'bg-orange-400/50' : 'bg-blue-500/50')
                        : 'bg-gray-700'
                  ]"
                />
              </div>
              <div class="flex items-center gap-3 mt-1.5 text-xs text-gray-600">
                <span>{{ progressPct(chain.tasks) }}% 完成</span>
                <span class="flex items-center gap-1"><span class="w-1.5 h-1.5 bg-blue-500 rounded-full inline-block"></span>🤖 Agent</span>
                <span class="flex items-center gap-1"><span class="w-1.5 h-1.5 bg-orange-400 rounded-full inline-block"></span>👤 人工</span>
              </div>
            </div>
            <span class="text-gray-500 text-sm transition-transform duration-200" :class="expanded.has(chain.chain_id) ? 'rotate-90' : ''">›</span>
          </div>

          <!-- Expanded task list -->
          <div v-if="expanded.has(chain.chain_id)" class="border-t border-gray-800">
            <div
              v-for="task in chain.tasks"
              :key="task.id"
              class="flex items-center gap-3 px-5 py-3 hover:bg-gray-800/50 border-b border-gray-800/50 last:border-0 cursor-pointer"
              @click="$router.push(`/tasks/${task.id}`)"
            >
              <span class="text-sm shrink-0">{{ taskIcon(task) }}</span>
              <div
                class="w-2 h-2 rounded-full shrink-0"
                :class="statusDot(task.status)"
              />
              <span class="text-sm text-gray-200 flex-1 truncate">{{ task.title }}</span>
              <span class="text-xs shrink-0" :class="statusColor(task.status)">{{ task.status }}</span>
              <span class="text-xs text-gray-600 shrink-0 w-20 text-right truncate">{{ task.assigned_to }}</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  </AppLayout>
</template>
