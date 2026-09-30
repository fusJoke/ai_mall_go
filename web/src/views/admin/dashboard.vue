<script setup lang="ts">
import { computed } from 'vue'
import { useAdminInfo } from '/@/stores/adminInfo'
import { useConfig } from '/@/stores/config'

// 注意：本视图所有 KPI 数值均为演示数据，非真实业务指标。
// 接入真实数据时，把下面 `kpis` 换成响应式 ref 即可，无需修改模板。
const kpis: ReadonlyArray<{ key: string; label: string; value: number; icon: string }> = [
    { key: 'orders', label: '今日订单', value: 128, icon: 'lucide:ShoppingBag' },
    { key: 'users', label: '用户总数', value: 3562, icon: 'lucide:Users' },
    { key: 'revenue', label: '营收（元）', value: 86420, icon: 'lucide:TrendingUp' },
]

const numberFormatter = new Intl.NumberFormat('zh-CN')

const adminInfo = useAdminInfo()
const config = useConfig()

// 昵称兜底：admin-init 还没跑完时也能正常显示。
const displayName = computed<string>(() => adminInfo.nickname || adminInfo.username || '管理员')

const siteConfig = computed(() => config.siteConfig)
</script>

<template>
    <div class="dashboard-page">
        <div class="greeting">欢迎回来，{{ displayName }}</div>

        <el-row :gutter="16">
            <el-col v-for="kpi in kpis" :key="kpi.key" :span="8">
                <el-card shadow="never" class="kpi-card">
                    <div class="kpi-header">
                        <Icon :name="kpi.icon" :size="20" />
                        <span class="kpi-label">{{ kpi.label }}</span>
                    </div>
                    <div class="kpi-value">{{ numberFormatter.format(kpi.value) }}</div>
                </el-card>
            </el-col>
        </el-row>

        <div class="info-footer">
            <div v-if="siteConfig.name" class="info-line">站点：{{ siteConfig.name }}</div>
            <div v-if="siteConfig.version" class="info-line">版本：{{ siteConfig.version }}</div>
        </div>
    </div>
</template>

<style scoped>
.dashboard-page {
    padding: 24px;
}
.greeting {
    font-size: 22px;
    font-weight: 500;
    margin-bottom: 24px;
    color: var(--el-text-color-primary);
}
.kpi-card {
    border-radius: 8px;
}
.kpi-header {
    display: flex;
    align-items: center;
    gap: 8px;
    color: var(--el-text-color-regular);
}
.kpi-label {
    font-size: 14px;
}
.kpi-value {
    font-size: 28px;
    font-weight: 600;
    color: var(--el-color-primary);
    margin-top: 8px;
}
.info-footer {
    margin-top: 32px;
    padding-top: 16px;
    border-top: 1px solid var(--el-border-color-lighter);
    color: var(--el-text-color-secondary);
    font-size: 13px;
}
.info-line + .info-line {
    margin-top: 4px;
}
</style>
