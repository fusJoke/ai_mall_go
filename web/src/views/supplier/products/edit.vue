<!-- src\views\supplier\products\edit.vue — 盲盒新建 / 编辑页（任务 6.10；5.3 批次化增强）。
     新建（无 :id）：表单 + 卡池条目编辑器（weight 加和校验 10000）+ 默认下架提示。
     编辑（有 :id）：基础信息 + lifecycle 分级 ——
       selling：可重写卡池（补库存/调概率；已开卖后端 409 pool_locked 锁定）；
       sold_out/settled：卡池已固化，仅可编辑基础信息。 -->
<template>
    <div class="page">
        <h2 class="title">{{ isEdit ? t('supplier.products.dialog.edit') : t('supplier.products.dialog.create') }}</h2>

        <el-alert
            v-if="!isEdit"
            :title="t('supplier.products.dialog.defaultOffSaleHint')"
            type="info"
            :closable="false"
            class="hint"
        />
        <el-alert
            v-else-if="poolEditable"
            :title="t('supplier.products.dialog.sellingPoolHint')"
            type="info"
            :closable="false"
            class="hint"
        />
        <el-alert
            v-else
            :title="t('supplier.products.dialog.lockedPoolHint')"
            type="warning"
            :closable="false"
            class="hint"
        />

        <el-form :model="form" label-width="140px" class="form">
            <el-form-item :label="t('supplier.products.dialog.name')" required>
                <el-input v-model="form.name" maxlength="100" />
            </el-form-item>
            <el-form-item :label="t('supplier.products.dialog.cover')">
                <el-input v-model="form.cover" placeholder="https://..." />
            </el-form-item>
            <el-form-item :label="t('supplier.products.dialog.price')" required>
                <el-input-number v-model="form.price" :min="0" :precision="2" :step="1" />
            </el-form-item>
            <el-form-item :label="t('supplier.products.dialog.description')">
                <el-input v-model="form.description" type="textarea" :rows="3" maxlength="500" />
            </el-form-item>

            <template v-if="poolEditable">
                <el-form-item :label="t('supplier.products.dialog.poolItems')">
                    <div class="pool-editor">
                        <div v-for="(item, idx) in form.items" :key="idx" class="pool-row">
                            <el-input-number v-model="item.card_id" :min="1" :controls="false" placeholder="Card ID" class="w-card" />
                            <el-select v-model="item.rarity" class="w-rarity">
                                <el-option label="SSR" value="SSR" />
                                <el-option label="SR" value="SR" />
                                <el-option label="R" value="R" />
                                <el-option label="N" value="N" />
                            </el-select>
                            <el-input-number v-model="item.weight" :min="0" :max="10000" :step="100" class="w-num" />
                            <el-input-number v-model="item.stock" :min="0" :max="999999" class="w-num" />
                            <el-button link type="danger" @click="form.items.splice(idx, 1)">✕</el-button>
                        </div>
                        <div class="pool-footer">
                            <el-button size="small" @click="addItem">{{ t('supplier.products.dialog.addPoolItem') }}</el-button>
                            <span :class="['weight-sum', weightSum === 10000 ? 'ok' : 'bad']">
                                {{ t('supplier.products.dialog.weightSum') }}: {{ weightSum }}
                            </span>
                        </div>
                        <p v-if="form.items.length > 0 && weightSum !== 10000" class="weight-tip">
                            {{ t('supplier.products.dialog.weightSumInvalid') }}
                        </p>
                    </div>
                </el-form-item>
            </template>

            <el-form-item>
                <el-button type="primary" :loading="saving" @click="save">{{ t('supplier.products.dialog.save') }}</el-button>
                <el-button @click="router.back()">{{ t('supplier.products.dialog.cancel') }}</el-button>
            </el-form-item>
        </el-form>
    </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import {
    supplierProductCreate,
    supplierProductDetail,
    supplierProductEdit,
    supplierProductUpdatePool,
    type PoolItemPayload,
    type SupplierBoxLifecycle,
} from '/@/api/supplier/product'
import { useSupplierInfo } from '/@/stores/supplier/supplierInfo'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const supplierInfo = useSupplierInfo()

const isEdit = computed(() => Boolean(route.params.id))
const saving = ref(false)
const editLifecycle = ref<SupplierBoxLifecycle>('selling')

/** 卡池编辑器可见性：新建 / selling 批次；sold_out/settled 固化不可改。 */
const poolEditable = computed(() => !isEdit.value || editLifecycle.value === 'selling')

const form = reactive({
    name: '',
    cover: '',
    price: 0,
    description: '',
    items: [] as PoolItemPayload[],
})

const weightSum = computed(() => form.items.reduce((acc, it) => acc + (Number(it.weight) || 0), 0))

function addItem() {
    form.items.push({ card_id: 1, rarity: 'N', weight: 0, stock: 0 })
}

/** 编辑态拉详情（box 基础信息 + 卡池条目预填；替代旧的全列表翻找）。 */
async function loadRow(id: number) {
    const { data } = await supplierProductDetail(id)
    const box = data.box
    form.name = box.name
    form.cover = box.cover
    form.price = Number(box.price)
    form.description = box.description
    editLifecycle.value = box.lifecycle
    form.items = (data.items ?? []).map((it) => ({
        card_id: Number(it.card_id),
        rarity: it.rarity,
        weight: Number(it.weight),
        stock: Number(it.stock),
    }))
}

function validatePool(): boolean {
    if (!poolEditable.value) return true
    if (form.items.length === 0) {
        ElMessage.warning(t('supplier.products.dialog.poolItems'))
        return false
    }
    if (weightSum.value !== 10000) {
        ElMessage.warning(t('supplier.products.dialog.weightSumInvalid'))
        return false
    }
    return true
}

/** 操作类错误提示；409 双码区分（后端 product.* 约定）。 */
function showActionError(err: unknown): boolean {
    const anyErr = err as { response?: { status?: number; data?: { code?: string; message?: string } } }
    if (anyErr?.response?.status === 401) {
        supplierInfo.reset()
        router.push('/supplier/login')
        return true
    }
    const code = anyErr?.response?.data?.code ?? ''
    const key =
        {
            product_pool_locked: 'supplier.products.dialog.poolLocked',
            product_lifecycle_forbidden: 'supplier.products.dialog.lockedPoolHint',
        }[code.replaceAll('.', '_')] ?? ''
    if (key) {
        ElMessage.error(t(key))
        return true
    }
    ElMessage.error(anyErr?.response?.data?.message || code || 'Error')
    return true
}

async function save() {
    saving.value = true
    try {
        if (isEdit.value) {
            const id = Number(route.params.id)
            // 卡池先于基础信息保存：改池失败（pool_locked 等）时不落基础信息，避免半套状态。
            if (poolEditable.value && !validatePool()) return
            if (poolEditable.value) {
                await supplierProductUpdatePool(id, form.items)
            }
            await supplierProductEdit({
                id,
                name: form.name || undefined,
                cover: form.cover || undefined,
                price: form.price,
                description: form.description || undefined,
            })
            ElMessage.success('OK')
        } else {
            if (!form.name) {
                ElMessage.warning(t('supplier.products.dialog.name'))
                return
            }
            if (!validatePool()) return
            await supplierProductCreate({
                name: form.name,
                cover: form.cover || undefined,
                price: form.price,
                description: form.description || undefined,
                items: form.items,
            })
            ElMessage.success('OK')
        }
        router.push('/supplier/products')
    } catch (err) {
        showActionError(err)
    } finally {
        saving.value = false
    }
}

onMounted(() => {
    if (!supplierInfo.token) {
        router.push('/supplier/login')
        return
    }
    if (isEdit.value) {
        loadRow(Number(route.params.id))
    } else {
        addItem()
    }
})
</script>

<style scoped lang="scss">
.page {
    background: #fff;
    border-radius: 16px;
    padding: 24px;
    max-width: 860px;
}
.title {
    margin: 0 0 20px;
    font-size: 20px;
    color: #0f172a;
}
.hint {
    margin-bottom: 16px;
}
.form {
    max-width: 640px;
}
.pool-editor {
    width: 100%;
}
.pool-row {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 8px;
}
.w-card {
    width: 110px;
}
.w-rarity {
    width: 90px;
}
.w-num {
    width: 120px;
}
.pool-footer {
    display: flex;
    align-items: center;
    gap: 16px;
}
.weight-sum {
    font-size: 13px;

    &.ok {
        color: #16a34a;
    }
    &.bad {
        color: #dc2626;
    }
}
.weight-tip {
    margin: 6px 0 0;
    font-size: 12px;
    color: #dc2626;
}
</style>
