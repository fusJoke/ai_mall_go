<!-- src\views\user\login.vue — C 端会员登录（任务 6.6）。
     复用点选验证码组件与 admin login 的提交流程；视觉用篮球橙蓝主题。 -->
<template>
    <div class="page">
        <div class="card">
            <div class="header">
                <span class="brand-icon">🏀</span>
                <h1>{{ t('user.login.title') }}</h1>
                <p>{{ t('user.login.subtitle') }}</p>
            </div>
            <form class="form" @submit.prevent="onSubmit">
                <div class="field">
                    <label for="user-login-username">{{ t('user.login.username') }}</label>
                    <input id="user-login-username" v-model="username" type="text" autocomplete="username" required />
                </div>
                <div class="field">
                    <label for="user-login-password">{{ t('user.login.password') }}</label>
                    <input id="user-login-password" v-model="password" type="password" autocomplete="current-password" required />
                </div>
                <label class="remember">
                    <input v-model="remember" type="checkbox" />
                    {{ t('user.login.remember') }}
                </label>
                <div v-if="errorMsg" class="error-msg">{{ errorMsg }}</div>
                <button type="submit" class="btn-primary" :disabled="loading">
                    {{ loading ? t('user.login.loading') : t('user.login.submit') }}
                </button>
            </form>
            <p class="alt-link">
                <RouterLink to="/supplier/login">{{ t('user.login.gotoSupplier') }}</RouterLink>
            </p>
        </div>
    </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import clickCaptcha from '/@/components/clickCaptcha/index'
import { shortUuid } from '/@/utils/random'
import { userLogin } from '/@/api/user/auth'
import { useUserInfo } from '/@/stores/user/userInfo'

const { t } = useI18n()
const router = useRouter()
const userInfo = useUserInfo()

const username = ref('')
const password = ref('')
const remember = ref(false)
const errorMsg = ref('')
const loading = ref(false)

/** 与 admin login 的 describeLoginError 同构：优先透出后端 message。 */
function describeLoginError(err: unknown): string {
    const anyErr = err as { response?: { data?: { code?: string; message?: string } }; message?: string } | undefined
    const code = anyErr?.response?.data?.code
    const map: Record<string, string> = {
        'login.invalid_credentials': t('user.login.invalidCredentials'),
        'login.invalid_captcha': t('user.login.invalidCaptcha'),
        'login.account_disabled': t('user.login.accountDisabled'),
    }
    if (code && map[code]) return map[code]
    if (anyErr?.message) return anyErr.message
    return t('user.login.failed')
}

async function submitLogin(captchaKey: string, points: { x: number; y: number }[]) {
    errorMsg.value = ''
    loading.value = true
    try {
        const { data } = await userLogin({
            username: username.value,
            password: password.value,
            captcha_key: captchaKey,
            points,
            remember: remember.value,
        })
        userInfo.setToken(data.token)
        userInfo.$patch({
            id: data.user.id,
            username: data.user.username,
            nickname: data.user.nickname,
            avatar: data.user.avatar,
            balance: data.user.balance,
        })
        router.push('/user/home')
    } catch (err) {
        errorMsg.value = describeLoginError(err)
    } finally {
        loading.value = false
    }
}

function onSubmit() {
    errorMsg.value = ''
    clickCaptcha(shortUuid(), (captchaKey, points) => {
        submitLogin(captchaKey, points)
    })
}
</script>

<style scoped lang="scss">
.page {
    min-height: 100vh;
    display: flex;
    align-items: center;
    justify-content: center;
    background: linear-gradient(135deg, #1e3a8a, #1e40af 60%, #f97316);
    padding: 24px;
}
.card {
    width: 100%;
    max-width: 400px;
    background: #fff;
    border-radius: 16px;
    padding: 40px 32px;
    box-shadow: 0 20px 50px rgba(15, 23, 42, 0.35);
}
.header {
    text-align: center;
    margin-bottom: 28px;

    .brand-icon {
        font-size: 40px;
    }
    h1 {
        margin: 8px 0 4px;
        font-size: 22px;
        color: #0f172a;
    }
    p {
        margin: 0;
        color: #64748b;
        font-size: 14px;
    }
}
.form {
    display: flex;
    flex-direction: column;
    gap: 16px;
}
.field {
    display: flex;
    flex-direction: column;
    gap: 6px;

    label {
        font-size: 13px;
        font-weight: 500;
        color: #334155;
    }
    input {
        height: 44px;
        padding: 0 12px;
        border-radius: 8px;
        border: 1px solid #cbd5e1;
        font-size: 14px;
        outline: none;

        &:focus {
            border-color: #f97316;
            box-shadow: 0 0 0 2px rgba(249, 115, 22, 0.2);
        }
    }
}
.remember {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 13px;
    color: #475569;
    cursor: pointer;
}
.error-msg {
    padding: 10px 12px;
    border-radius: 8px;
    background: rgba(127, 29, 29, 0.08);
    border: 1px solid rgba(127, 29, 29, 0.15);
    color: #dc2626;
    font-size: 13px;
}
.btn-primary {
    height: 44px;
    border: none;
    border-radius: 8px;
    background: linear-gradient(90deg, #f97316, #ea580c);
    color: #fff;
    font-size: 15px;
    font-weight: 600;
    cursor: pointer;

    &:hover {
        opacity: 0.92;
    }
    &:disabled {
        opacity: 0.5;
        cursor: not-allowed;
    }
}
.alt-link {
    margin-top: 20px;
    text-align: center;
    font-size: 13px;

    a {
        color: #1e40af;
        text-decoration: none;

        &:hover {
            text-decoration: underline;
        }
    }
}
</style>
