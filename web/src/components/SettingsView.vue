<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'

import { api } from '../api'
import { copyText, errorMessage } from '../format'
import { loadOverview, pushToast, state } from '../store'

const form = reactive({
  name: '',
  slug: '',
  subtitle: '',
  description: '',
  tintColor: '#5B7CFA',
  cacheSeconds: 600,
})

const saving = ref(false)
const copied = ref(false)

watch(
  () => state.overview?.source,
  (source) => {
    if (!source) {
      return
    }
    Object.assign(form, {
      name: source.name,
      slug: source.slug,
      subtitle: source.subtitle,
      description: source.description,
      tintColor: source.tintColor,
      cacheSeconds: source.cacheSeconds,
    })
  },
  { immediate: true },
)

const sourceUrl = computed(() => state.overview?.source.url ?? '')

async function save(): Promise<void> {
  if (saving.value) {
    return
  }
  saving.value = true
  try {
    await api.saveSource({
      name: form.name,
      slug: form.slug,
      subtitle: form.subtitle,
      description: form.description,
      tintColor: form.tintColor,
      cacheSeconds: Number(form.cacheSeconds) || 0,
    })
    await loadOverview(true)
    pushToast('源设置已保存。')
  } catch (error) {
    pushToast(errorMessage(error), 'error')
  } finally {
    saving.value = false
  }
}

async function copy(): Promise<void> {
  if (await copyText(sourceUrl.value)) {
    copied.value = true
    window.setTimeout(() => {
      copied.value = false
    }, 1600)
  }
}
</script>

<template>
  <div class="stack">
    <header class="page-head">
      <span class="eyebrow">工作台</span>
      <h1>设置</h1>
      <p class="muted">源的展示信息，以及 Release 数据的刷新频率。</p>
    </header>

    <div v-if="!state.overview" class="stack">
      <div class="skel skel--row"></div>
      <div class="skel skel--row"></div>
    </div>

    <template v-else>
      <div class="settings-grid">
        <section class="panel panel--identity">
          <header class="panel__head">
            <h2>源信息</h2>
            <p class="muted">会在 SideStore 和 AltStore 的源顶部显示。</p>
          </header>

          <div class="form-grid">
            <label class="field">
              <span>源名称</span>
              <input v-model="form.name" class="input" type="text" />
            </label>

            <label class="field">
              <span>源 slug</span>
              <input v-model="form.slug" class="input" type="text" />
              <small class="field__hint">
                文件名里 <span class="mono">/</span> 后面的部分。填 <span class="mono">source</span> 时
                <span class="mono">/source.json</span> 与 <span class="mono">/{{ form.slug }}.json</span> 都可用。
              </small>
            </label>

            <div class="form-row">
              <label class="field">
                <span>强调色</span>
                <div class="color-field">
                  <input v-model="form.tintColor" class="input" type="text" />
                  <input
                    v-model="form.tintColor"
                    class="color-field__picker"
                    type="color"
                    aria-label="选择强调色"
                  />
                </div>
              </label>

              <label class="field">
                <span>缓存秒数</span>
                <input v-model.number="form.cacheSeconds" class="input" type="number" min="0" step="30" />
                <small class="field__hint">仓库 Release 数据的缓存时长。</small>
              </label>
            </div>
          </div>
        </section>

        <section class="panel panel--desc">
          <header class="panel__head">
            <h2>描述</h2>
            <p class="muted">纯文本，客户端不会渲染 Markdown。</p>
          </header>

          <div class="form-grid">
            <label class="field">
              <span>副标题</span>
              <input v-model="form.subtitle" class="input" type="text" />
            </label>
            <label class="field">
              <span>描述</span>
              <textarea v-model="form.description" class="input textarea" rows="4"></textarea>
              <small class="field__hint">这一行会作为源的简介显示在客户端里。</small>
            </label>
          </div>
        </section>

        <section class="panel panel--endpoint">
          <header class="panel__head">
            <h2>源地址</h2>
            <p class="muted">填到 SideStore 或 AltStore 里的就是这一条。</p>
          </header>

          <div class="endpoint">
            <span class="mono endpoint__url">{{ sourceUrl }}</span>
            <button class="btn btn--ghost btn--sm" type="button" @click="copy">
              {{ copied ? '已复制' : '复制' }}
            </button>
          </div>
          <p class="muted panel__note">
            客户端会按自己的节奏缓存这个 JSON，改完设置不必手动通知它们。
          </p>
          <div class="steps">
            <p class="steps__title">在客户端里添加</p>
            <ol>
              <li>复制上面的地址</li>
              <li>打开 SideStore，进入 Sources</li>
              <li>点右上角 +，粘贴地址后确认</li>
            </ol>
          </div>
        </section>

        <section class="panel panel--env">
          <header class="panel__head">
            <h2>服务器环境</h2>
            <p class="muted">这些值来自容器环境变量。</p>
          </header>

          <dl class="facts">
            <div class="fact">
              <dt>对外地址</dt>
              <dd class="mono">{{ state.overview.baseUrl }}</dd>
            </div>
            <div class="fact">
              <dt>后台路径</dt>
              <dd class="mono">{{ state.overview.adminPath }}</dd>
            </div>
            <div class="fact">
              <dt>GitHub Token</dt>
              <dd>
                <span class="chip" :data-status="state.overview.githubTokenConfigured ? 'ok' : 'pending'">
                  {{ state.overview.githubTokenConfigured ? '已配置' : '未设置' }}
                </span>
                <small class="field__hint">
                  不配置时每小时 60 次请求，配置后 5000 次。设置
                  <span class="mono">GITHUB_TOKEN</span> 后重启容器生效。
                </small>
              </dd>
            </div>
            <div class="fact">
              <dt>后台访问</dt>
              <dd>
                <span class="chip" :data-status="state.overview.authRequired ? 'ok' : 'no_ipa'">
                  {{ state.overview.authRequired ? '需要密码' : '无需登录' }}
                </span>
                <small class="field__hint">
                  设置 <span class="mono">ADMIN_TOKEN</span> 可以给这个后台加密码；无论是否设置，
                  源 JSON 都是公开的。
                </small>
              </dd>
            </div>
          </dl>
        </section>
      </div>

      <div class="settings-actions">
        <span class="muted">改动会在下一次请求源时生效。</span>
        <button class="btn btn--primary" type="button" :disabled="saving" @click="save">
          {{ saving ? '保存中…' : '保存设置' }}
        </button>
      </div>
    </template>
  </div>
</template>

