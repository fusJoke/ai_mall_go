<template>
    <el-drawer v-model="visible" :title="$t('layouts.profile')" size="410px" :append-to-body="true">
        <div class="ba-account">
            <!-- 资料概览 -->
            <div class="account-base">
                <el-avatar :size="70">{{ adminInfo.nickname.slice(0, 1) }}</el-avatar>
                <div class="account-other">
                    <div class="account-username">{{ adminInfo.username }}</div>
                    <div class="account-lasttime">{{ adminInfo.last_login_at || $t('layouts.lastLoginTime') + ': -' }}</div>
                </div>
            </div>

            <!-- 资料编辑（示例数据约定：仅写本地 store，不落服务端） -->
            <el-form ref="formRef" :model="form" :rules="formRules" label-width="80px">
                <el-form-item :label="$t('layouts.nickname')" prop="nickname">
                    <el-input v-model="form.nickname" :placeholder="$t('layouts.nickname')" />
                </el-form-item>
                <el-form-item :label="$t('layouts.avatar')">
                    <agUpload type="image" :model-value="form.avatar" :return-full-url="true" @update:model-value="onAvatarChange" />
                </el-form-item>
            </el-form>

            <el-alert :title="$t('layouts.accountLocalOnlyTip')" type="info" :closable="false" show-icon />

            <div class="account-footer">
                <el-button type="primary" :loading="submitting" @click="onSubmit">{{ $t('layouts.save') }}</el-button>
            </div>
        </div>
    </el-drawer>
</template>

<script setup lang="ts">
import { reactive, ref, watch } from 'vue'
import { ElMessage, type FormInstance, type FormRules } from 'element-plus'
import { useI18n } from 'vue-i18n'
import agUpload from '/@/components/agInput/components/agUpload.vue'
import { useAdminInfo } from '/@/stores/adminInfo'

/**
 * 管理员账号资料抽屉（对齐 buildadmin baAccount.vue 的精简移植）：
 * 展示当前管理员概览 + 昵称 / 头像编辑，头像走 agUpload（支持多驱动上传）。
 * 提交仅更新本地 adminInfo store（示例数据约定，服务端接口后续接入）。
 */
const { t } = useI18n()
const adminInfo = useAdminInfo()

const visible = defineModel<boolean>({ default: false })

const submitting = ref(false)
const formRef = ref<FormInstance>()

const form = reactive({
    nickname: '',
    avatar: '',
})

const formRules = reactive<FormRules>({
    nickname: [{ required: true, message: () => t('layouts.nickname'), trigger: 'blur' }],
})

watch(visible, (val) => {
    if (val) {
        form.nickname = adminInfo.nickname
        form.avatar = adminInfo.avatar ?? ''
    }
})

const onAvatarChange = (val: string | string[]) => {
    form.avatar = Array.isArray(val) ? (val[0] ?? '') : val
}

const onSubmit = async () => {
    if (!formRef.value) return
    try {
        await formRef.value.validate()
    } catch {
        return
    }
    submitting.value = true
    try {
        adminInfo.dataFill({
            nickname: form.nickname,
            avatar: form.avatar,
        })
        ElMessage.success(t('layouts.save'))
        visible.value = false
    } finally {
        submitting.value = false
    }
}
</script>

<style scoped lang="scss">
.ba-account {
    padding: 0 4px;
}
.account-base {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 4px 0 20px;

    .account-username {
        font-weight: 600;
        margin-bottom: 4px;
    }
    .account-lasttime {
        font-size: 12px;
        color: var(--el-text-color-secondary);
    }
}
.account-footer {
    display: flex;
    justify-content: center;
    padding: 16px 0 10px;
}
</style>
