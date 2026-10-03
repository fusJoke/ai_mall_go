<!-- src\views\supplier\seckill\edit.vue — 新建 / 编辑秒杀活动（任务 12.5）。
     :id 缺省 = 新建（表单含时间窗 + 卡池校验提示）；有 :id = 编辑（仅秒杀价 / 限购，
     时间窗与盲盒后端拒绝修改）。total_stock 提示不得超过卡池剩余库存（后端 422 兜底）。 -->
<template>
    <div class="page">
        <RouterLink to="/supplier/seckill" class="back">← {{ t('supplier.seckill.backToList') }}</RouterLink>
        <h2 class="title">{{ editId ? t('supplier.seckill.editTitle') : t('supplier.seckill.createTitle') }}</h2>

        <el-form :model="form" label-width="140px" class="form">
            <el-form-item v-if="!editId" :label="t('supplier.seckill.form.blindBoxId')" required>
                <el-input-number v-model="form.blind_box_id" :min="1" :disabled="saving" />
            </el-form-item>
            <el-form-item v-else :label="t('supplier.seckill.form.blindBoxId')">
                <el-input :model-value="String(editing?.blind_box_id ?? '')" disabled />
            </el-form-item>

            <el-form-item :label="t('supplier.seckill.form.seckillPrice')" required>
                <el-input-number v-model="form.seckill_price" :min="0.01" :precision="2" :disabled="saving" />
                <span class="hint">{{ t('supplier.seckill.form.priceHint') }}</span>
            </el-form-item>

            <el-form-item v-if="!editId" :label="t('supplier.seckill.form.totalStock')" required>
                <el-input-number v-model="form.total_stock" :min="1" :disabled="saving" />
                <span class="hint">{{ t('supplier.seckill.form.stockHint') }}</span>
            </el-form-item>

            <el-form-item :label="t('supplier.seckill.form.perUserLimit')" required>
                <el-input-number v-model="form.per_user_limit" :min="1" :disabled="saving" />
            </el-form-item>

            <el-form-item v-if="!editId" :label="t('supplier.seckill.form.timeRange')" required>
                <el-date-picker
                    v-model="form.timeRange"
                    type="datetimerange"
                    value-format="YYYY-MM-DDTHH:mm:ssZ"
                    :disabled="saving"
                />
            </el-form-item>
            <el-form-item v-else :label="t('supplier.seckill.form.timeRange')">
                <el-input :model-value="timeWindow" disabled />
                <span class="hint">{{ t('supplier.seckill.form.windowHint') }}</span>
            </el-form-item>

            <el-form-item>
                <el-button type="primary" :loading="saving" @click="save">{{ t('supplier.seckill.form.save') }}</el-button>
                <el-button @click="router.push('/supplier/seckill')">{{ t('supplier.seckill.form.cancel') }}</el-button>
            </el-form-item>
        </el-form>
    </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import {
    supplierSeckillCreate,
    supplierSeckillEdit,
    supplierSeckillList,
    type SupplierSeckill,
} from '/@/api/supplier/seckill'
import { useSupplierInfo } from '/@/stores/supplier/supplierInfo'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const supplierInfo = useSupplierInfo()

const editId = ref<number | null>(null)
const editing = ref<SupplierSeckill | null>(null)
const saving = ref(false)

const form = reactive({
    blind_box_id: 1,
    seckill_price: 0.01,
    total_stock: 100,
    per_user_limit: 1,
    timeRange: [] as string[],
})

const timeWindow = computed(() =>
    editing.value ? `${new Date(editing.value.start_at).toLocaleString()} ~ ${new Date(editing.value.end_at).toLocaleString()}` : '',
)

/** 秒杀业务错误码 → 文案（与 handler/supplier/seckill.go 的 mapSeckillErr 对齐）。 */
function describeSeckillError(err: unknown): string {
    const anyErr = err as { response?: { status?: number; data?: { code?: string } } }
    const map: Record<string, string> = {
        'seckill.invalid_time_window': t('supplier.seckill.invalidTimeWindow'),
        'seckill.invalid_price': t('supplier.seckill.invalidPrice'),
        'seckill.stock_exceeds_pool': t('supplier.seckill.stockExceedsPool'),
        'seckill.forbidden': t('supplier.seckill.forbidden'),
        'seckill.not_found': t('supplier.seckill.notFound'),
        'seckill.redis_init_failed': t('supplier.seckill.redisInitFailed'),
    }
    const code = anyErr?.response?.data?.code ?? ''
    return map[code] ?? t('supplier.seckill.failed')
}

async function loadEditing() {
    // 后端无 get-by-id 端点，从列表拉一页找到目标行（B 端量级足够）。
    const id = Number(route.params.id)
    if (!id) return
    try {
        const { data } = await supplierSeckillList({ page: 1, page_size: 200 })
        const found = (data.items ?? []).find((it) => it.id === id) ?? null
        if (!found) {
            ElMessage.error(t('supplier.seckill.notFound'))
            router.push('/supplier/seckill')
            return
        }
        editing.value = found
        form.seckill_price = Number(found.seckill_price)
        form.per_user_limit = found.per_user_limit
    } catch (err) {
        handle401(err)
        ElMessage.error(t('supplier.seckill.failed'))
    }
}

async function save() {
    if (!editId.value) {
        if (form.timeRange.length !== 2) {
            ElMessage.warning(t('supplier.seckill.invalidTimeWindow'))
            return
        }
    }
    if (form.seckill_price <= 0 || form.per_user_limit <= 0) {
        ElMessage.warning(t('supplier.seckill.invalidPrice'))
        return
    }

    saving.value = true
    try {
        if (editId.value) {
            await supplierSeckillEdit({
                id: editId.value,
                seckill_price: form.seckill_price,
                per_user_limit: form.per_user_limit,
            })
        } else {
            await supplierSeckillCreate({
                blind_box_id: form.blind_box_id,
                seckill_price: form.seckill_price,
                total_stock: form.total_stock,
                per_user_limit: form.per_user_limit,
                start_at: form.timeRange[0],
                end_at: form.timeRange[1],
            })
        }
        ElMessage.success('OK')
        router.push('/supplier/seckill')
    } catch (err) {
        const anyErr = err as { response?: { status?: number } }
        if (anyErr?.response?.status === 401) {
            supplierInfo.reset()
            router.push('/supplier/login')
            return
        }
        ElMessage.error(describeSeckillError(err))
    } finally {
        saving.value = false
    }
}

function handle401(err: unknown) {
    const anyErr = err as { response?: { status?: number } }
    if (anyErr?.response?.status === 401) {
        supplierInfo.reset()
        router.push('/supplier/login')
    }
}

onMounted(() => {
    if (!supplierInfo.token) {
        router.push('/supplier/login')
        return
    }
    if (route.params.id) {
        editId.value = Number(route.params.id)
        loadEditing()
    }
})
</script>

<style scoped lang="scss">
.page {
    background: #fff;
    border-radius: 16px;
    padding: 24px;
}
.back {
    color: #1e40af;
    text-decoration: none;
    font-size: 14px;

    &:hover {
        text-decoration: underline;
    }
}
.title {
    margin: 12px 0 20px;
    font-size: 20px;
    color: #0f172a;
}
.form {
    max-width: 640px;
}
.hint {
    margin-left: 12px;
    font-size: 12px;
    color: #94a3b8;
}
</style>
