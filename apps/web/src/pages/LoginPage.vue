<template>
  <q-layout view="hHh Lpr lff">
    <q-page-container>
      <q-page class="sci-fi-login">
        <!-- 透视网格背景 -->
        <div class="sci-fi-bg-grid" />
        <!-- 中心径向辉光 -->
        <div class="sci-fi-bg-glow" />
        <!-- 浮动粒子层 -->
        <div class="sci-fi-bg-particles">
          <span v-for="i in 18" :key="i" class="sci-fi-particle" :style="particleStyle(i)" />
        </div>

        <!-- HUD 四角装饰 -->
        <div class="sci-fi-hud sci-fi-hud--tl">
          <span class="sci-fi-hud__label">SYS://FRAMEWORK</span>
          <span class="sci-fi-hud__coord">N 31°14' E 121°28'</span>
        </div>
        <div class="sci-fi-hud sci-fi-hud--tr">
          <span class="sci-fi-hud__label">{{ t('auth.systemStatus') }}</span>
          <span class="sci-fi-hud__value">
            <span class="sci-fi-led sci-fi-led--green" />
            {{ t('auth.systemOnline') }}
          </span>
        </div>
        <div class="sci-fi-hud sci-fi-hud--bl">
          <span class="sci-fi-hud__label">v0.1.0 · BUILD 20260624</span>
        </div>
        <div class="sci-fi-hud sci-fi-hud--br">
          <span class="sci-fi-hud__label">{{ t('auth.secureChannel') }}</span>
          <span class="sci-fi-hud__value">
            <span class="sci-fi-led sci-fi-led--cyan" />
            TLS 1.3
          </span>
        </div>

        <!-- ===== 左侧：网络拓扑 ===== -->
        <div class="topo-panel">
          <div class="topo-header">
            <div class="topo-header__title">
              <q-icon name="account_tree" size="18px" class="q-mr-xs" />
              {{ t('auth.topologyTitle') }}
            </div>
            <div class="topo-header__subtitle">{{ t('auth.topologySubtitle') }}</div>
          </div>

          <div class="topo-canvas">
            <!-- 拓扑连线 SVG -->
            <svg class="topo-svg" viewBox="0 0 600 520" preserveAspectRatio="xMidYMid meet">
              <!-- Cloud → Core -->
              <path class="topo-edge" d="M300,80 L300,135" />
              <!-- Core → DB -->
              <path class="topo-edge topo-edge--curve" d="M280,185 Q200,225 130,255" />
              <!-- Core → App -->
              <path class="topo-edge" d="M300,185 L300,255" />
              <!-- Core → Web -->
              <path class="topo-edge topo-edge--curve" d="M320,185 Q400,225 470,255" />
              <!-- DB → PC1 -->
              <path class="topo-edge topo-edge--curve" d="M115,295 Q90,345 80,400" />
              <!-- App → PC2 -->
              <path class="topo-edge topo-edge--curve" d="M280,295 Q255,345 235,400" />
              <!-- App → Terminal -->
              <path class="topo-edge topo-edge--curve" d="M320,295 Q345,345 365,400" />
              <!-- Web → IoT -->
              <path class="topo-edge topo-edge--curve" d="M485,295 Q510,345 520,400" />
              <!-- 数据流动光点 -->
              <circle class="topo-packet" r="3">
                <animateMotion dur="2s" repeatCount="indefinite" path="M300,80 L300,135" />
              </circle>
              <circle class="topo-packet" r="3">
                <animateMotion dur="2.5s" repeatCount="indefinite" path="M280,185 Q200,225 130,255" />
              </circle>
              <circle class="topo-packet" r="3">
                <animateMotion dur="2s" repeatCount="indefinite" path="M300,185 L300,255" />
              </circle>
              <circle class="topo-packet" r="3">
                <animateMotion dur="2.5s" repeatCount="indefinite" path="M320,185 Q400,225 470,255" />
              </circle>
              <circle class="topo-packet" r="3">
                <animateMotion dur="3s" repeatCount="indefinite" path="M115,295 Q90,345 80,400" />
              </circle>
              <circle class="topo-packet" r="3">
                <animateMotion dur="2.8s" repeatCount="indefinite" path="M280,295 Q255,345 235,400" />
              </circle>
              <circle class="topo-packet" r="3">
                <animateMotion dur="3s" repeatCount="indefinite" path="M320,295 Q345,345 365,400" />
              </circle>
              <circle class="topo-packet" r="3">
                <animateMotion dur="2.6s" repeatCount="indefinite" path="M485,295 Q510,345 520,400" />
              </circle>
            </svg>

            <!-- 拓扑节点 -->
            <div class="topo-node topo-node--cloud" style="left: 50%; top: 15.4%">
              <div class="topo-node__icon"><q-icon name="cloud" size="22px" /></div>
              <span class="topo-node__label">{{ t('auth.nodeCloud') }}</span>
              <span class="topo-node__led" />
            </div>
            <div class="topo-node topo-node--core" style="left: 50%; top: 35.6%">
              <div class="topo-node__icon"><q-icon name="router" size="22px" /></div>
              <span class="topo-node__label">{{ t('auth.nodeCore') }}</span>
              <span class="topo-node__led" />
            </div>
            <div class="topo-node topo-node--db" style="left: 21.7%; top: 55.6%">
              <div class="topo-node__icon"><q-icon name="storage" size="20px" /></div>
              <span class="topo-node__label">{{ t('auth.nodeDB') }}</span>
              <span class="topo-node__led" />
            </div>
            <div class="topo-node topo-node--app" style="left: 50%; top: 55.6%">
              <div class="topo-node__icon"><q-icon name="dns" size="20px" /></div>
              <span class="topo-node__label">{{ t('auth.nodeApp') }}</span>
              <span class="topo-node__led" />
            </div>
            <div class="topo-node topo-node--web" style="left: 78.3%; top: 55.6%">
              <div class="topo-node__icon"><q-icon name="language" size="20px" /></div>
              <span class="topo-node__label">{{ t('auth.nodeWeb') }}</span>
              <span class="topo-node__led" />
            </div>
            <div class="topo-node topo-node--pc" style="left: 13.3%; top: 80%">
              <div class="topo-node__icon"><q-icon name="computer" size="18px" /></div>
              <span class="topo-node__label">{{ t('auth.nodePC') }}-01</span>
              <span class="topo-node__led" />
            </div>
            <div class="topo-node topo-node--pc" style="left: 39.2%; top: 80%">
              <div class="topo-node__icon"><q-icon name="computer" size="18px" /></div>
              <span class="topo-node__label">{{ t('auth.nodePC') }}-02</span>
              <span class="topo-node__led" />
            </div>
            <div class="topo-node topo-node--terminal" style="left: 60.8%; top: 80%">
              <div class="topo-node__icon"><q-icon name="terminal" size="18px" /></div>
              <span class="topo-node__label">{{ t('auth.nodeTerminal') }}</span>
              <span class="topo-node__led" />
            </div>
            <div class="topo-node topo-node--iot" style="left: 86.7%; top: 80%">
              <div class="topo-node__icon"><q-icon name="sensors" size="18px" /></div>
              <span class="topo-node__label">{{ t('auth.nodeIoT') }}</span>
              <span class="topo-node__led" />
            </div>
          </div>
        </div>

        <!-- ===== 右侧：登录卡片 ===== -->
        <div class="login-panel">
          <q-card flat class="sci-fi-card">
            <!-- 扫描线动画 -->
            <div class="sci-fi-scanline" />
            <!-- 卡片四角括号 -->
            <span class="sci-fi-corner sci-fi-corner--tl" />
            <span class="sci-fi-corner sci-fi-corner--tr" />
            <span class="sci-fi-corner sci-fi-corner--bl" />
            <span class="sci-fi-corner sci-fi-corner--br" />

            <!-- 标题区 -->
            <div class="sci-fi-header">
              <div class="sci-fi-header__icon">
                <q-icon name="hub" size="32px" />
              </div>
              <div class="sci-fi-title">{{ loginTitle }}</div>
              <div class="sci-fi-subtitle">{{ t('auth.welcomeSubtitle') }}</div>
            </div>

            <!-- 表单区 -->
            <div class="sci-fi-form">
              <template v-if="step === 'login'">
              <q-input
                v-model.trim="identity"
                dense
                outlined
                clearable
                class="sci-fi-input"
                :placeholder="t('auth.usernameLabel')"
                autocomplete="username"
                @keyup.enter="submit"
              >
                <template #prepend>
                  <q-icon name="person" size="xs" />
                </template>
              </q-input>

              <q-input
                v-model="password"
                dense
                outlined
                class="sci-fi-input"
                :type="showPwd ? 'text' : 'password'"
                :placeholder="t('auth.passwordLabel')"
                autocomplete="current-password"
                @keyup.enter="submit"
              >
                <template #prepend>
                  <q-icon name="lock" size="xs" />
                </template>
                <template #append>
                  <q-btn
                    flat
                    round
                    dense
                    :icon="showPwd ? 'visibility_off' : 'visibility'"
                    tabindex="-1"
                    @click.stop="showPwd = !showPwd"
                  />
                </template>
              </q-input>

              <!-- 记住密码 -->
              <div class="sci-fi-remember">
                <q-checkbox v-model="rememberMe" :label="t('auth.rememberMe')" dense class="sci-fi-checkbox" />
              </div>

              <q-banner v-if="localError" rounded dense class="sci-fi-error q-mt-sm">
                <template #avatar>
                  <q-icon name="error_outline" />
                </template>
                {{ localError }}
              </q-banner>

              <!-- 历史登录 -->
              <div v-if="loginHistory.length > 0" class="sci-fi-history">
                <div class="sci-fi-history__title">
                  <q-icon name="history" size="xs" />
                  {{ t('auth.loginHistory') }}
                </div>
                <div class="sci-fi-history__list">
                  <button
                    v-for="name in loginHistory"
                    :key="name"
                    class="sci-fi-history__chip"
                    @click="useHistory(name)"
                  >
                    <q-icon name="person" size="xs" />
                    {{ name }}
                  </button>
                </div>
              </div>
              </template>

              <!-- MFA 二步验证 -->
              <template v-else>
                <div class="sci-fi-mfa">
                  <div class="sci-fi-mfa__icon">
                    <q-icon name="phonelink_lock" size="28px" />
                  </div>
                  <div class="sci-fi-mfa__title">{{ t('mfa.stepTitle') }}</div>
                  <div class="sci-fi-mfa__hint">{{ t('mfa.stepHint') }}</div>
                  <q-input
                    v-model.trim="mfaCode"
                    dense
                    outlined
                    class="sci-fi-input"
                    :placeholder="t('mfa.placeholder')"
                    maxlength="6"
                    input-class="text-center"
                    @keyup.enter="submitMFA"
                  >
                    <template #prepend>
                      <q-icon name="vpn_key" size="xs" />
                    </template>
                  </q-input>
                  <q-btn
                    flat
                    dense
                    no-caps
                    size="xs"
                    color="primary"
                    :label="t('common.cancel')"
                    @click="backToLogin"
                  />
                </div>
              </template>
            </div>

            <!-- 操作区 -->
            <div class="sci-fi-actions">
              <q-btn
                unelevated
                :loading="auth.loginLoading"
                :label="step === 'mfa' ? t('mfa.verify') : t('auth.submit')"
                padding="sm lg"
                class="sci-fi-btn"
                @click="step === 'mfa' ? submitMFA() : submit()"
              >
                <template #loading>
                  <q-spinner-dots />
                </template>
              </q-btn>
            </div>

            <!-- 底部状态条 -->
            <div class="sci-fi-status-bar">
              <span class="sci-fi-status-dot" />
              <span class="sci-fi-status-text">{{ t('auth.secureChannel') }}</span>
              <span class="sci-fi-status-sep">·</span>
              <span class="sci-fi-status-text sci-fi-status-text--muted">{{ t('auth.backendHint') }}</span>
            </div>
          </q-card>
        </div>
      </q-page>
    </q-page-container>
  </q-layout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from 'vue';
import { useRouter, useRoute } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { Dark } from 'quasar';
import { useAuthStore } from '../stores/auth';
import { cachedSystemName } from '../stores/system-name';

const { t } = useI18n();
const router = useRouter();
const route = useRoute();
const auth = useAuthStore();

// 登录页未登录、无权限读 system_configs：沿用最近一次登录缓存的系统名称，兜底 i18n
const loginTitle = computed(() => cachedSystemName() || t('auth.welcomeTitle'));

// 登录页科幻风格专为暗色设计：进入时强制 dark，离开时恢复用户偏好。
let savedDark: boolean | null = null;

const REMEMBER_KEY = 'framework_remember';
const HISTORY_KEY = 'framework_login_history';

const identity = ref('');
const password = ref('');
const mfaCode = ref('');
const showPwd = ref(false);
const rememberMe = ref(false);
const localError = ref('');
const loginHistory = ref<string[]>([]);
type LoginStep = 'login' | 'mfa';
const step = ref<LoginStep>('login');

// 粒子样式生成
function particleStyle(i: number) {
  const left = (i * 37) % 100;
  const top = (i * 53) % 100;
  const size = 2 + (i % 3);
  const delay = (i * 0.7) % 8;
  const duration = 6 + (i % 5);
  return {
    left: `${left}%`,
    top: `${top}%`,
    width: `${size}px`,
    height: `${size}px`,
    animationDelay: `${delay}s`,
    animationDuration: `${duration}s`,
  };
}

onMounted(() => {
  // 登录页科幻风格专为暗色设计，进入时强制 dark 模式（不写入 localStorage）
  savedDark = Dark.isActive;
  if (!savedDark) Dark.set(true);

  // 页签标题与登录页标题同口径（从主界面退出跳回时 title 已是该值，幂等）
  document.title = loginTitle.value;

  // 仅加载记住的用户名（不存储明文密码，安全考虑）
  try {
    const saved = localStorage.getItem(REMEMBER_KEY);
    if (saved) {
      const obj = JSON.parse(saved);
      identity.value = obj.username || '';
      rememberMe.value = true;
    }
  } catch {
    // ignore
  }
  loadHistory();
});

onBeforeUnmount(() => {
  // 离开登录页时恢复用户实际主题偏好
  if (savedDark !== null) Dark.set(savedDark);
});

function loadHistory() {
  try {
    const raw = localStorage.getItem(HISTORY_KEY);
    loginHistory.value = raw ? JSON.parse(raw) : [];
  } catch {
    loginHistory.value = [];
  }
}

function addHistory(username: string) {
  const list = loginHistory.value.filter((u) => u !== username);
  list.unshift(username);
  loginHistory.value = list.slice(0, 5);
  localStorage.setItem(HISTORY_KEY, JSON.stringify(loginHistory.value));
}

function useHistory(username: string) {
  identity.value = username;
  password.value = '';
}

async function submit() {
  localError.value = '';
  // clearable 的 q-input 清空后 model 为 null，归一化防空指针
  const id = (identity.value ?? '').trim();
  if (!id || !password.value) {
    localError.value = t('auth.loginFailedCredentials');
    return;
  }
  try {
    await auth.login(
      'username',
      id,
      password.value,
    );
    await onLoginSuccess(id);
  } catch (err: unknown) {
    // MFA 二步：auth.login 抛出 mfaRequired 信号时切换到 MFA step
    const e = err as Error & { mfaRequired?: boolean };
    if (e?.mfaRequired) {
      step.value = 'mfa';
      localError.value = '';
      return;
    }
    localError.value = t('auth.loginFailed');
  }
}

async function submitMFA() {
  localError.value = '';
  if (!mfaCode.value || mfaCode.value.length !== 6) {
    localError.value = t('mfa.invalid');
    return;
  }
  try {
    await auth.verifyMFA(mfaCode.value);
    const id = (identity.value ?? '').trim();
    await onLoginSuccess(id);
  } catch {
    localError.value = t('mfa.invalid');
  }
}

function backToLogin() {
  step.value = 'login';
  mfaCode.value = '';
  localError.value = '';
}

async function onLoginSuccess(id: string) {
  // 保存/清除记住的用户名（不存储明文密码）
  if (rememberMe.value) {
    localStorage.setItem(REMEMBER_KEY, JSON.stringify({ username: id }));
  } else {
    localStorage.removeItem(REMEMBER_KEY);
  }
  addHistory(id);
  const redirect = (route.query.redirect as string) || '/dashboard';
  await router.push(redirect);
}
</script>

<style lang="sass">
// ===== 科幻登录页 — 左拓扑 / 右登录 双栏布局 =====

.sci-fi-login
  position: relative
  min-height: 100vh
  overflow: hidden
  display: flex
  align-items: stretch
  background: var(--gradient-page)

// ----- 背景层 -----

.sci-fi-bg-grid
  position: absolute
  inset: 0
  background-image: linear-gradient(var(--glass-border) 1px, transparent 1px), linear-gradient(90deg, var(--glass-border) 1px, transparent 1px)
  background-size: 48px 48px
  opacity: 0.3
  mask-image: radial-gradient(ellipse at center, rgba(0, 0, 0, 1) 30%, transparent 75%)
  -webkit-mask-image: radial-gradient(ellipse at center, rgba(0, 0, 0, 1) 30%, transparent 75%)
  animation: sci-fi-grid-drift 20s linear infinite

body.body--dark .sci-fi-bg-grid
  opacity: 0.45
  background-image: linear-gradient(color-mix(in srgb, var(--color-accent) 18%, transparent) 1px, transparent 1px), linear-gradient(90deg, color-mix(in srgb, var(--color-accent) 18%, transparent) 1px, transparent 1px)

@keyframes sci-fi-grid-drift
  from
    background-position: 0 0, 0 0
  to
    background-position: 48px 48px, 48px 48px

.sci-fi-bg-glow
  position: absolute
  top: 50%
  left: 35%
  width: 600px
  height: 600px
  transform: translate(-50%, -50%)
  background: radial-gradient(circle, color-mix(in srgb, var(--color-accent) 12%, transparent) 0%, transparent 60%)
  pointer-events: none
  animation: sci-fi-glow-pulse 4s ease-in-out infinite

body.body--dark .sci-fi-bg-glow
  background: radial-gradient(circle, color-mix(in srgb, var(--color-accent) 18%, transparent) 0%, transparent 65%)

@keyframes sci-fi-glow-pulse
  0%, 100%
    opacity: 0.6
    transform: translate(-50%, -50%) scale(1)
  50%
    opacity: 1
    transform: translate(-50%, -50%) scale(1.08)

.sci-fi-bg-particles
  position: absolute
  inset: 0
  pointer-events: none

.sci-fi-particle
  position: absolute
  border-radius: 50%
  background: var(--color-accent)
  opacity: 0
  animation: sci-fi-particle-float linear infinite

body.body--dark .sci-fi-particle
  box-shadow: 0 0 6px var(--color-accent)

@keyframes sci-fi-particle-float
  0%
    opacity: 0
    transform: translateY(0) scale(0.5)
  20%
    opacity: 0.7
  80%
    opacity: 0.7
  100%
    opacity: 0
    transform: translateY(-120px) scale(1.2)

// ----- HUD 角落装饰 -----

.sci-fi-hud
  position: absolute
  display: flex
  flex-direction: column
  gap: 2px
  padding: 12px 16px
  font-size: 10px
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace
  color: var(--color-text-tertiary)
  z-index: 2
  letter-spacing: 0.05em

.sci-fi-hud--tl
  top: 0
  left: 0

.sci-fi-hud--tr
  top: 0
  right: 0
  align-items: flex-end

.sci-fi-hud--bl
  bottom: 0
  left: 0

.sci-fi-hud--br
  bottom: 0
  right: 0
  align-items: flex-end

.sci-fi-hud__label
  text-transform: uppercase
  opacity: 0.6

.sci-fi-hud__value
  display: flex
  align-items: center
  gap: 4px
  font-weight: 700
  color: var(--color-text-secondary)

.sci-fi-led
  display: inline-block
  width: 8px
  height: 8px
  border-radius: 50%
  animation: sci-fi-led-blink 2s ease-in-out infinite

.sci-fi-led--green
  background: var(--q-positive)
  box-shadow: 0 0 6px var(--q-positive)

.sci-fi-led--cyan
  background: var(--color-accent)
  box-shadow: 0 0 6px var(--color-accent)

@keyframes sci-fi-led-blink
  0%, 100%
    opacity: 1
  50%
    opacity: 0.4

// ===== 左侧：网络拓扑面板 =====

.topo-panel
  flex: 1
  display: flex
  flex-direction: column
  padding: 60px 40px 60px 60px
  z-index: 1
  min-width: 0

.topo-header
  margin-bottom: 16px

.topo-header__title
  display: flex
  align-items: center
  font-size: 16px
  font-weight: 800
  letter-spacing: 0.04em
  color: var(--color-text-heading)
  text-transform: uppercase

  .q-icon
    color: var(--color-accent)

.topo-header__subtitle
  margin-top: 4px
  font-size: 11px
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace
  color: var(--color-text-tertiary)
  letter-spacing: 0.06em

.topo-canvas
  position: relative
  flex: 1
  min-height: 400px

.topo-svg
  position: absolute
  inset: 0
  width: 100%
  height: 100%

// 拓扑连线
.topo-edge
  fill: none
  stroke: color-mix(in srgb, var(--color-accent) 35%, var(--glass-border))
  stroke-width: 1.5
  stroke-dasharray: 6 4
  opacity: 0.5
  animation: topo-flow 1.2s linear infinite

body.body--dark .topo-edge
  stroke: color-mix(in srgb, var(--color-accent) 50%, transparent)
  opacity: 0.6

@keyframes topo-flow
  to
    stroke-dashoffset: -10

// 数据流动光点
.topo-packet
  fill: var(--color-accent)
  filter: drop-shadow(0 0 4px var(--color-accent))
  opacity: 0.8

body.body--dark .topo-packet
  filter: drop-shadow(0 0 6px var(--color-accent))

// 拓扑节点
.topo-node
  position: absolute
  transform: translate(-50%, -50%)
  display: flex
  flex-direction: column
  align-items: center
  gap: 4px
  z-index: 2

.topo-node__icon
  display: flex
  align-items: center
  justify-content: center
  width: 44px
  height: 44px
  border-radius: 12px
  background: color-mix(in srgb, var(--glass-elevated) 80%, transparent)
  border: 1px solid color-mix(in srgb, var(--color-accent) 20%, var(--glass-border))
  color: var(--color-text-secondary)
  backdrop-filter: blur(8px)
  -webkit-backdrop-filter: blur(8px)
  transition: border-color 0.2s ease, box-shadow 0.2s ease, transform 0.2s ease

  &:hover
    transform: scale(1.1)
    border-color: var(--color-accent)
    box-shadow: 0 0 16px color-mix(in srgb, var(--color-accent) 25%, transparent)

body.body--dark .topo-node__icon
  background: color-mix(in srgb, var(--glass-surface) 60%, transparent)

// 不同节点类型配色
.topo-node--cloud .topo-node__icon
  border-color: color-mix(in srgb, var(--color-accent) 40%, var(--glass-border))
  color: var(--color-accent)

.topo-node--core .topo-node__icon
  width: 52px
  height: 52px
  border-color: var(--color-accent)
  color: var(--color-accent)
  box-shadow: 0 0 12px color-mix(in srgb, var(--color-accent) 20%, transparent)

.topo-node--db .topo-node__icon,
.topo-node--app .topo-node__icon,
.topo-node--web .topo-node__icon
  color: var(--color-text-heading)

.topo-node__label
  font-size: 10px
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace
  color: var(--color-text-tertiary)
  white-space: nowrap
  letter-spacing: 0.03em

.topo-node__led
  position: absolute
  top: 4px
  right: 4px
  width: 6px
  height: 6px
  border-radius: 50%
  background: var(--q-positive)
  box-shadow: 0 0 4px var(--q-positive)
  animation: sci-fi-led-blink 2s ease-in-out infinite

// ===== 右侧：登录面板 =====

.login-panel
  flex: 0 0 580px
  display: flex
  align-items: center
  justify-content: center
  padding: 40px 80px 40px 20px
  position: relative
  z-index: 3

// 登录卡片
.sci-fi-card
  position: relative
  width: 100%
  max-width: 480px
  border-radius: 20px
  background: var(--glass-elevated)
  border: 1px solid color-mix(in srgb, var(--color-accent) 20%, var(--glass-border))
  box-shadow: 0 0 0 1px color-mix(in srgb, var(--color-accent) 8%, transparent), 0 0 24px color-mix(in srgb, var(--color-accent) 12%, transparent), 0 8px 32px rgba(0, 0, 0, 0.12)
  backdrop-filter: blur(var(--glass-blur-elevated))
  -webkit-backdrop-filter: blur(var(--glass-blur-elevated))
  overflow: hidden
  animation: sci-fi-card-in 0.6s cubic-bezier(0.22, 1, 0.36, 1) both

body.body--dark .sci-fi-card
  box-shadow: 0 0 0 1px color-mix(in srgb, var(--color-accent) 15%, transparent), 0 0 32px color-mix(in srgb, var(--color-accent) 18%, transparent), 0 8px 32px rgba(0, 0, 0, 0.4)

@keyframes sci-fi-card-in
  from
    opacity: 0
    transform: translateY(16px) scale(0.96)
  to
    opacity: 1
    transform: translateY(0) scale(1)

// 扫描线
.sci-fi-scanline
  position: absolute
  top: 0
  left: 0
  right: 0
  height: 2px
  background: linear-gradient(90deg, transparent, var(--color-accent), transparent)
  opacity: 0.6
  animation: sci-fi-scan 3s ease-in-out infinite
  pointer-events: none
  z-index: 4

@keyframes sci-fi-scan
  0%
    top: 0
    opacity: 0
  10%
    opacity: 0.8
  90%
    opacity: 0.8
  100%
    top: 100%
    opacity: 0

// 卡片四角括号
.sci-fi-corner
  position: absolute
  width: 20px
  height: 20px
  border: 2px solid var(--color-accent)
  pointer-events: none
  z-index: 3

.sci-fi-corner--tl
  top: 10px
  left: 10px
  border-right: none
  border-bottom: none
  border-top-left-radius: 6px

.sci-fi-corner--tr
  top: 10px
  right: 10px
  border-left: none
  border-bottom: none
  border-top-right-radius: 6px

.sci-fi-corner--bl
  bottom: 10px
  left: 10px
  border-right: none
  border-top: none
  border-bottom-left-radius: 6px

.sci-fi-corner--br
  bottom: 10px
  right: 10px
  border-left: none
  border-top: none
  border-bottom-right-radius: 6px

// 标题区
.sci-fi-header
  text-align: center
  padding: 28px 24px 16px

.sci-fi-header__icon
  display: inline-flex
  align-items: center
  justify-content: center
  width: 56px
  height: 56px
  margin-bottom: 12px
  border-radius: 16px
  background: color-mix(in srgb, var(--color-accent) 12%, var(--glass-surface))
  border: 1px solid color-mix(in srgb, var(--color-accent) 25%, var(--glass-border))
  color: var(--color-accent)
  box-shadow: 0 0 16px color-mix(in srgb, var(--color-accent) 20%, transparent)

body.body--dark .sci-fi-header__icon
  box-shadow: 0 0 24px color-mix(in srgb, var(--color-accent) 30%, transparent)

.sci-fi-title
  font-size: 22px
  font-weight: 800
  letter-spacing: 0.02em
  color: var(--color-text-heading)
  text-shadow: 0 0 12px color-mix(in srgb, var(--color-accent) 20%, transparent)

body.body--dark .sci-fi-title
  text-shadow: 0 0 16px color-mix(in srgb, var(--color-accent) 35%, transparent)

.sci-fi-subtitle
  margin-top: 4px
  font-size: 11px
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace
  letter-spacing: 0.1em
  text-transform: uppercase
  color: var(--color-text-tertiary)

// 表单区
.sci-fi-form
  padding: 8px 24px 0

.sci-fi-input
  margin-bottom: 14px

  .q-field__control
    border-radius: 10px
    background: color-mix(in srgb, var(--glass-surface) 50%, transparent)

  &.q-field--focused .q-field__control
    border-color: var(--color-accent)
    box-shadow: 0 0 0 2px color-mix(in srgb, var(--color-accent) 15%, transparent), 0 0 12px color-mix(in srgb, var(--color-accent) 15%, transparent)

body.body--dark .sci-fi-input
  .q-field__control
    background: color-mix(in srgb, var(--glass-surface) 35%, transparent)

  &.q-field--focused .q-field__control
    box-shadow: 0 0 0 2px color-mix(in srgb, var(--color-accent) 20%, transparent), 0 0 16px color-mix(in srgb, var(--color-accent) 25%, transparent)

// 记住密码
.sci-fi-remember
  display: flex
  align-items: center
  margin-bottom: 8px

.sci-fi-checkbox
  .q-checkbox__label
    font-size: 12px
    color: var(--color-text-secondary)

  &.q-checkbox--dark .q-checkbox__inner--truthfully .q-checkbox__bg
    color: var(--color-accent)

// 历史登录
.sci-fi-history
  margin-top: 12px
  padding-top: 12px
  border-top: 1px dashed var(--glass-border)

.sci-fi-history__title
  display: flex
  align-items: center
  gap: 4px
  font-size: 10px
  font-weight: 700
  text-transform: uppercase
  letter-spacing: 0.06em
  color: var(--color-text-tertiary)
  margin-bottom: 8px

  .q-icon
    color: var(--color-accent)

.sci-fi-history__list
  display: flex
  flex-wrap: wrap
  gap: 6px

.sci-fi-history__chip
  display: inline-flex
  align-items: center
  gap: 4px
  padding: 4px 10px
  border-radius: 999px
  border: 1px solid color-mix(in srgb, var(--color-accent) 20%, var(--glass-border))
  background: color-mix(in srgb, var(--glass-surface) 60%, transparent)
  color: var(--color-text-secondary)
  font-size: 11px
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace
  cursor: pointer
  transition: border-color 0.15s ease, background 0.15s ease, transform 0.15s ease

  .q-icon
    color: var(--color-accent)
    opacity: 0.7

  &:hover
    border-color: var(--color-accent)
    background: color-mix(in srgb, var(--color-accent) 8%, var(--glass-elevated))
    transform: translateY(-1px)

    .q-icon
      opacity: 1

// MFA 二步验证
.sci-fi-mfa
  display: flex
  flex-direction: column
  align-items: center
  padding: 12px 0 4px

.sci-fi-mfa__icon
  display: flex
  align-items: center
  justify-content: center
  width: 48px
  height: 48px
  margin-bottom: 12px
  border-radius: 12px
  background: color-mix(in srgb, var(--color-accent) 12%, var(--glass-surface))
  border: 1px solid color-mix(in srgb, var(--color-accent) 25%, var(--glass-border))
  color: var(--color-accent)

.sci-fi-mfa__title
  font-size: 15px
  font-weight: 700
  color: var(--color-text-heading)
  margin-bottom: 4px

.sci-fi-mfa__hint
  font-size: 11px
  color: var(--color-text-tertiary)
  text-align: center
  margin-bottom: 16px

.sci-fi-error
  background: color-mix(in srgb, var(--negative, var(--q-negative)) 12%, transparent)
  border: 1px solid color-mix(in srgb, var(--negative, var(--q-negative)) 30%, transparent)
  color: var(--negative, var(--q-negative))

// 操作区
.sci-fi-actions
  padding: 4px 24px 16px

.sci-fi-btn
  width: 100%
  border-radius: 10px
  background: var(--color-accent)
  color: var(--q-dark)
  font-weight: 700
  letter-spacing: 0.05em
  text-transform: uppercase
  font-size: 13px
  box-shadow: 0 0 16px color-mix(in srgb, var(--color-accent) 30%, transparent)
  transition: box-shadow 0.2s ease, transform 0.15s ease

  &:hover
    box-shadow: 0 0 24px color-mix(in srgb, var(--color-accent) 45%, transparent)
    transform: translateY(-1px)

  &:active
    transform: translateY(0)

body.body--dark .sci-fi-btn
  box-shadow: 0 0 20px color-mix(in srgb, var(--color-accent) 40%, transparent)

  &:hover
    box-shadow: 0 0 32px color-mix(in srgb, var(--color-accent) 55%, transparent)

// 底部状态条
.sci-fi-status-bar
  display: flex
  align-items: center
  gap: 6px
  padding: 12px 24px 16px
  border-top: 1px solid var(--glass-border)
  font-size: 10px
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace
  color: var(--color-text-tertiary)

.sci-fi-status-dot
  display: inline-block
  width: 6px
  height: 6px
  border-radius: 50%
  background: var(--color-accent)
  box-shadow: 0 0 6px var(--color-accent)
  animation: sci-fi-led-blink 2s ease-in-out infinite

.sci-fi-status-text
  letter-spacing: 0.03em

.sci-fi-status-text--muted
  opacity: 0.6

.sci-fi-status-sep
  opacity: 0.4

// ----- 响应式 -----

@media (max-width: 960px)
  .topo-panel
    display: none

  .login-panel
    flex: 1
    padding: 40px 20px

@media (max-width: 600px)
  .sci-fi-hud
    font-size: 9px
    padding: 8px 10px

  .sci-fi-header
    padding: 20px 20px 12px

  .sci-fi-form,
  .sci-fi-actions,
  .sci-fi-status-bar
    padding-left: 20px
    padding-right: 20px
</style>
