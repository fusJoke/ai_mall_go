<script setup lang="ts">
/**
 * P7 登录页。
 *
 * 手机号 + 短信验证码。本期由本地 mock 承接（边界见 `src/mock/index.ts`）。
 *
 * **滑块验证码已从本页移除**（产品决策）：滑块能力本身（`components/slider-captcha.vue`、
 * `api/auth.ts` 的 createSlider / verifySlider、`mock/slider.ts` 与两条 mock 路由、
 * `SliderCreateResponse` / `SliderVerifyResponse` 类型）**全部保留备用**，只是登录页不再接入 ——
 * 重新接入时把组件挂回表单、恢复 `canSubmit` 的 `captcha` 门控、并在提交体里带上
 * `captcha_key` / `captcha_x` / `captcha_w` / `captcha_h` 即可。
 *
 * 登录成功后若配置了真实令牌（`VITE_DEV_TOKEN`），会用它替换 mock token ——
 * 使后续抽卡 / 订单真正打到库里；未配置时会话降级并在「我的」页提示。
 */

import { computed, onUnmounted, ref } from 'vue'
import { onLoad } from '@dcloudio/uni-app'

import NavBar from '@/components/nav-bar.vue'
import { loginMobile, sendSms } from '@/api/auth'
import { MOBILE_RE, SMS_COUNTDOWN, TAB_PATHS, USE_MOCK } from '@/config'
// FIXED_SMS_CODE 只用于页面底部的「开发期提示」文案（v-if="USE_MOCK"），
// 因此从 mock 层内部取，而不是从通用 config 取（OCR 审查 P2-5）。
import { FIXED_SMS_CODE } from '@/mock/constants'
import { setAuth } from '@/store/user'
import { applyDevToken } from '@/utils/session'
import { toast, toastError } from '@/utils/toast'
import type { ApiError } from '@/types'

const mobile = ref('')
const smsCode = ref('')
const countdown = ref(0)
const submitting = ref(false)
const redirect = ref('')

let timer: ReturnType<typeof setInterval> | null = null

const canSubmit = computed(
  () => MOBILE_RE.test(mobile.value) && smsCode.value.trim().length >= 4 && !submitting.value
)

/** 只接受站内路径，其余一律丢弃（防开放重定向）。 */
function safeRedirect(raw?: string): string {
  if (!raw) return ''
  let decoded = raw
  try {
    decoded = decodeURIComponent(raw)
  } catch {
    return ''
  }
  if (!decoded.startsWith('/') || decoded.startsWith('//')) return ''
  return decoded
}

function startCountdown(): void {
  stopCountdown()
  countdown.value = SMS_COUNTDOWN
  timer = setInterval(() => {
    countdown.value -= 1
    if (countdown.value <= 0) stopCountdown()
  }, 1000)
}

function stopCountdown(): void {
  if (timer) {
    clearInterval(timer)
    timer = null
  }
  countdown.value = 0
}

async function onSendSms(): Promise<void> {
  if (countdown.value > 0) return
  if (!MOBILE_RE.test(mobile.value)) {
    toastError('手机号格式不正确')
    return
  }
  try {
    await sendSms(mobile.value)
    toast('验证码已发送')
    startCountdown()
  } catch (err) {
    toastError((err as ApiError).message || '网络异常，请稍后重试')
  }
}

/** 登录成功后的落点：优先回跳，其次回上一页，最后回首页。 */
function leave(): void {
  const target = redirect.value
  if (!target) {
    const pages = getCurrentPages()
    if (pages.length > 1) {
      uni.navigateBack()
      return
    }
    uni.switchTab({ url: '/pages/home/index' })
    return
  }

  const path = target.split('?')[0]
  if (TAB_PATHS.includes(path)) {
    uni.switchTab({ url: path })
    return
  }
  uni.redirectTo({ url: target })
}

async function onSubmit(): Promise<void> {
  if (!canSubmit.value) return
  submitting.value = true
  try {
    const res = await loginMobile({
      mobile: mobile.value,
      sms_code: smsCode.value.trim(),
    })

    // 写入会话；mock 模式下若有真实令牌则替换，保证需鉴权接口可用。
    setAuth(res.token, res.user)
    applyDevToken(res.user)

    toast('登录成功')
    setTimeout(leave, 400)
  } catch (err) {
    toastError((err as ApiError).message || '网络异常，请稍后重试')
  } finally {
    submitting.value = false
  }
}

onLoad((query) => {
  redirect.value = safeRedirect((query as Record<string, string>)?.redirect)
})

onUnmounted(stopCountdown)
</script>

<template>
  <view class="page login">
    <NavBar title="欢迎回来" show-back />

    <view class="head">
      <text class="head__title">欢迎回来</text>
      <text class="head__sub">手机号登录，未注册将自动创建账号</text>
    </view>

    <view class="form">
      <view class="field">
        <text class="field__label">+86</text>
        <input
          v-model="mobile"
          class="field__input"
          type="number"
          :maxlength="11"
          placeholder="请输入手机号"
          placeholder-class="field__ph"
        />
      </view>

      <view class="field">
        <input
          v-model="smsCode"
          class="field__input"
          type="number"
          :maxlength="6"
          placeholder="请输入验证码"
          placeholder-class="field__ph"
        />
        <view
          class="field__action"
          :class="{ 'field__action--disabled': countdown > 0 }"
          @tap="onSendSms"
        >
          <text>{{ countdown > 0 ? `${countdown}秒后重发` : '获取验证码' }}</text>
        </view>
      </view>

      <view
        class="btn btn--primary btn--block submit"
        :class="{ 'btn--disabled': !canSubmit }"
        @tap="onSubmit"
      >
        <text>{{ submitting ? '登录中' : '登录' }}</text>
      </view>

      <text v-if="USE_MOCK" class="dev-tip">
        开发期固定验证码 · {{ FIXED_SMS_CODE }}
      </text>
    </view>
  </view>
</template>

<style lang="scss" scoped>
.login {
  padding: 0 var(--space-md);
}

.head {
  padding: var(--space-xl) 0 var(--space-lg);
}

.head__title {
  display: block;
  font-size: 48rpx;
  font-weight: 700;
  color: var(--text);
}

.head__sub {
  display: block;
  margin-top: var(--space-xs);
  font-size: 26rpx;
  color: var(--muted);
}

.field {
  display: flex;
  align-items: center;
  height: 96rpx;
  padding: 0 var(--space-md);
  margin-bottom: var(--space-md);
  border-radius: var(--radius-md);
  background-color: var(--surface);
  border: 1rpx solid var(--border);
}

.field__label {
  font-size: 28rpx;
  color: var(--muted);
  margin-right: var(--space-sm);
}

.field__input {
  flex: 1;
  font-size: 30rpx;
  color: var(--text);
  height: 100%;
}

.field__ph {
  color: var(--dim);
}

.field__action {
  padding-left: var(--space-sm);
  border-left: 1rpx solid var(--border);
  font-size: 26rpx;
  color: var(--gold);
}

.field__action--disabled {
  color: var(--dim);
}

.submit {
  margin-top: var(--space-sm);
}

.dev-tip {
  display: block;
  margin-top: var(--space-md);
  text-align: center;
  font-size: 22rpx;
  color: var(--dim);
}
</style>
