<template>
    <div class="page">
        <div class="left" :style="{ background: `linear-gradient(135deg, ${primaryColor}e6, ${primaryColor}, ${primaryColor}cc)` }">
            <div class="brand">
                <div class="brand-icon">
                    <Sparkles :size="16" />
                </div>
                <span>{{ brandName }}</span>
            </div>
            <div class="characters-area">
                <AnimatedCharacters
                    :is-typing="isTyping"
                    :has-secret="!!password"
                    :secret-visible="showPassword"
                />
            </div>
            <div class="footer-links">
                <a href="#">隐私政策</a>
                <a href="#">服务条款</a>
                <a href="#">联系我们</a>
            </div>
            <div class="deco-grid" />
            <div class="deco-circle deco-circle-1" />
            <div class="deco-circle deco-circle-2" />
        </div>
        <div class="right">
            <div class="form-wrapper">
                <div class="mobile-brand">
                    <div class="brand-icon">
                        <Sparkles :size="16" />
                    </div>
                    <span>{{ brandName }}</span>
                </div>
                <div class="header">
                    <h1>{{ title }}</h1>
                    <p>{{ subtitle }}</p>
                </div>
                <form class="form" @submit.prevent="onSubmit">
                    <div class="field">
                        <label for="login-username">用户名</label>
                        <input
                            id="login-username"
                            v-model="username"
                            type="text"
                            autocomplete="username"
                            required
                            :placeholder="usernamePlaceholder"
                            @focus="isTyping = true"
                            @blur="isTyping = false"
                        />
                    </div>
                    <div class="field">
                        <label for="login-password">密码</label>
                        <div class="password-wrap">
                            <input
                                id="login-password"
                                v-model="password"
                                :type="showPassword ? 'text' : 'password'"
                                autocomplete="current-password"
                                required
                                placeholder="••••••••"
                            />
                            <button type="button" class="eye-btn" @click="showPassword = !showPassword">
                                <EyeOff v-if="showPassword" :size="20" />
                                <Eye v-else :size="20" />
                            </button>
                        </div>
                    </div>
                    <div class="options">
                        <label class="remember">
                            <input v-model="remember" type="checkbox" />
                            记住我 30 天
                        </label>
                        <a href="#" class="forgot">忘记密码？</a>
                    </div>
                    <div v-if="errorMsg" class="error-msg">{{ errorMsg }}</div>
                    <button
                        type="submit"
                        class="btn-primary"
                        :disabled="loading"
                        :style="{ background: primaryColor }"
                    >
                        {{ loading ? '登录中...' : '登录' }}
                    </button>
                </form>
                <button v-if="showGoogleLogin" type="button" class="btn-google">
                    <Mail :size="20" />
                    使用 Google 账号登录
                </button>
                <p class="signup-link">
                    还没有账号？<a href="#">立即注册</a>
                </p>
            </div>
        </div>
    </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { Eye, EyeOff, Mail, Sparkles } from '@lucide/vue'
import AnimatedCharacters from './components/AnimatedCharacters.vue'
import { login } from '/@/api/admin'
import clickCaptcha from '/@/components/clickCaptcha/index'
import { useAdminInfo } from '/@/stores/adminInfo'
import { shortUuid } from '/@/utils/random'

withDefaults(defineProps<{
    brandName?: string
    title?: string
    subtitle?: string
    usernamePlaceholder?: string
    primaryColor?: string
    showGoogleLogin?: boolean
}>(), {
    brandName: 'AI Go Mall',
    title: '欢迎回来',
    subtitle: '请输入您的账号信息',
    usernamePlaceholder: '请输入用户名',
    primaryColor: '#4f46e5',
    showGoogleLogin: false,
})

const router = useRouter()
const adminInfo = useAdminInfo()

const showPassword = ref(false)
const username = ref('')
const password = ref('')
const remember = ref(false)
const errorMsg = ref('')
const loading = ref(false)
const isTyping = ref(false)

/** 暴露给父组件用于远程控制错误/loading 状态 */
defineExpose({
    setError: (msg: string) => {
        errorMsg.value = msg
    },
    setLoading: (v: boolean) => {
        loading.value = v
    },
})

/**
 * 把 axios 错误对象翻译成对用户友好的中文文案。
 *
 * axios 默认文案像 "Request failed with status code 401" 用户看不懂；
 * 后端返回的 {code, message} 字段会通过拦截器塞到 err.message 上（参见
 * /web/src/utils/request.ts 的 axios 拦截器），优先使用 message，否则 fallback。
 */
function describeLoginError(err: unknown): string {
    const anyErr = err as { response?: { data?: { message?: string } }; message?: string } | undefined
    const fromBackend = anyErr?.response?.data?.message
    if (fromBackend) return fromBackend
    if (anyErr?.message) return anyErr.message
    return '登录失败，请重试'
}

async function submitLogin(captchaKey: string, points: { x: number; y: number }[]) {
    errorMsg.value = ''
    loading.value = true
    try {
        const { data } = await login({
            username: username.value,
            password: password.value,
            captcha_key: captchaKey,
            points,
            remember: remember.value,
        })

        // 1) 先存 token
        adminInfo.setToken(data.token)
        // 2) 再填其余字段（dataFill 默认 exclude token，避免被覆盖）
        adminInfo.dataFill({
            id: data.admin.id,
            username: data.admin.username,
            nickname: data.admin.nickname,
            avatar: data.admin.avatar,
            last_login_at: data.admin.last_login_at ?? '',
            last_login_ip: data.admin.last_login_ip ?? '',
            super: false,
        })

        // 3) 跳转 /admin —— /admin redirect 到 /admin/loading，由 loading 视图决定下一步
        await router.push('/admin')
    } catch (err) {
        errorMsg.value = describeLoginError(err)
    } finally {
        loading.value = false
    }
}

function onSubmit() {
    errorMsg.value = ''
    // 先弹点选验证码：用户点完 → 预检通过 → 回调拿到 captchaKey + points → 真正调 login。
    clickCaptcha(shortUuid(), (captchaKey, points) => {
        submitLogin(captchaKey, points)
    })
}
</script>

<style scoped lang="scss">
.page {
    display: grid;
    grid-template-columns: 1fr 1fr;
    min-height: 100vh;
}
.left {
    position: relative;
    display: flex;
    flex-direction: column;
    justify-content: space-between;
    padding: 48px;
    color: white;
    overflow: hidden;
}
.brand {
    position: relative;
    z-index: 20;
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 18px;
    font-weight: 600;
}
.brand-icon {
    width: 32px;
    height: 32px;
    border-radius: 8px;
    background: rgba(255, 255, 255, 0.1);
    backdrop-filter: blur(8px);
    display: flex;
    align-items: center;
    justify-content: center;
}
.characters-area {
    position: relative;
    z-index: 20;
    display: flex;
    align-items: flex-end;
    justify-content: center;
    height: 500px;
}
.footer-links {
    position: relative;
    z-index: 20;
    display: flex;
    gap: 32px;
    font-size: 14px;
}
.footer-links a {
    color: rgba(255, 255, 255, 0.6);
    transition: color 0.2s;

    &:hover {
        color: white;
    }
}
.deco-grid {
    position: absolute;
    inset: 0;
    background-image:
        linear-gradient(to right, rgba(255, 255, 255, 0.05) 1px, transparent 1px),
        linear-gradient(to bottom, rgba(255, 255, 255, 0.05) 1px, transparent 1px);
    background-size: 20px 20px;
}
.deco-circle {
    position: absolute;
    border-radius: 50%;
    filter: blur(48px);
}
.deco-circle-1 {
    top: 25%;
    right: 25%;
    width: 256px;
    height: 256px;
    background: rgba(255, 255, 255, 0.1);
}
.deco-circle-2 {
    bottom: 25%;
    left: 25%;
    width: 384px;
    height: 384px;
    background: rgba(255, 255, 255, 0.05);
}
.right {
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 32px;
    background: #fff;
}
.form-wrapper {
    width: 100%;
    max-width: 420px;
}
.mobile-brand {
    display: none;
}
.header {
    text-align: center;
    margin-bottom: 40px;

    h1 {
        font-size: 30px;
        font-weight: 700;
        letter-spacing: -0.5px;
        margin-bottom: 8px;
    }
    p {
        color: #71717a;
        font-size: 14px;
    }
}
.form {
    display: flex;
    flex-direction: column;
    gap: 20px;
}
.field {
    display: flex;
    flex-direction: column;
    gap: 8px;

    label {
        font-size: 14px;
        font-weight: 500;
    }
    input {
        height: 48px;
        padding: 0 12px;
        border-radius: 6px;
        border: 1px solid #d4d4d8;
        font-size: 14px;
        outline: none;
        background: #fff;
        transition: border-color 0.2s;

        &:focus {
            border-color: #4f46e5;
            box-shadow: 0 0 0 2px rgba(79, 70, 229, 0.2);
        }
    }
}
.password-wrap {
    position: relative;

    input {
        width: 100%;
        padding-right: 40px;
    }
}
.eye-btn {
    position: absolute;
    right: 12px;
    top: 50%;
    transform: translateY(-50%);
    background: none;
    border: none;
    color: #a1a1aa;
    transition: color 0.2s;

    &:hover {
        color: #3f3f46;
    }
}
.options {
    display: flex;
    align-items: center;
    justify-content: space-between;
    font-size: 14px;
}
.remember {
    display: flex;
    align-items: center;
    gap: 8px;
    cursor: pointer;

    input {
        width: 16px;
        height: 16px;
        accent-color: #4f46e5;
    }
}
.forgot {
    color: #4f46e5;
    font-weight: 500;

    &:hover {
        text-decoration: underline;
    }
}
.error-msg {
    padding: 12px;
    font-size: 14px;
    color: #f87171;
    background: rgba(127, 29, 29, 0.08);
    border: 1px solid rgba(127, 29, 29, 0.15);
    border-radius: 8px;
}
.btn-primary {
    width: 100%;
    height: 48px;
    font-size: 16px;
    font-weight: 500;
    border: none;
    border-radius: 6px;
    color: white;
    transition: opacity 0.2s;

    &:hover {
        opacity: 0.9;
    }
    &:disabled {
        opacity: 0.5;
        cursor: not-allowed;
    }
}
.btn-google {
    width: 100%;
    height: 48px;
    margin-top: 24px;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 8px;
    border-radius: 6px;
    border: 1px solid #d4d4d8;
    background: #fff;
    font-size: 14px;
    transition: background 0.2s;

    &:hover {
        background: #fafafa;
    }
}
.signup-link {
    text-align: center;
    font-size: 14px;
    color: #71717a;
    margin-top: 32px;

    a {
        color: #18181b;
        font-weight: 500;

        &:hover {
            text-decoration: underline;
        }
    }
}
@media (max-width: 1023px) {
    .page {
        grid-template-columns: 1fr;
    }
    .left {
        display: none;
    }
    .mobile-brand {
        display: flex;
        align-items: center;
        justify-content: center;
        gap: 8px;
        font-size: 18px;
        font-weight: 600;
        margin-bottom: 48px;
    }
}
</style>
