<!-- src\views\supplier\promotions\index.vue — 供应商限时特价管理。
     任务清单（6.10）未列本页，但 spec 要求供应商可管理自家活动（后端 API 已就位），
     补齐闭环（ledger 有裁定）：表格 + 新建对话框 + 启停 / 改价 / 删除。 -->
<template>
    <div class="page">
        <div class="toolbar">
            <h2 class="title">{{ t('supplier.promotions.title') }}</h2>
            <el-button type="primary" @click="openCreate">{{ t('supplier.promotions.create') }}</el-button>
        </div>

        <el-table v-loading="loading" :data="items" stripe>
            <el-table-column prop="id" :label="t('supplier.promotions.columns.id')" width="64" />
            <el-table-column prop="blind_box_id" :label="t('supplier.promotions.columns.blindBox')" width="100" />
            <el-table-column :label="t('supplier.promotions.columns.originalPrice')" width="110">
                <template #default="{ row }">¥{{ Number(row.original_price).toFixed(2) }}</template>
            </el-table-column>
            <el-table-column :label="t('supplier.promotions.columns.promoPrice')" width="110">
                <template #default="{ row }">¥{{ Number(row.promo_price).toFixed(2) }}</template>
            </el-table-column>
            <el-table-column :label="t('supplier.promotions.columns.startAt')" min-width="150">
                <template #default="{ row }">{{ new Date(row.start_at).toLocaleString() }}</template>
            </el-table-column>
            <el-table-column :label="t('supplier.promotions.columns.endAt')" min-width="150">
                <template #default="{ row }">{{ new Date(row.end_at).toLocaleString() }}</template>
            </el-table-column>
            <el-table-column :label="t('supplier.promotions.columns.status')" width="100">
                <template #default="{ row }">
                    <el-tag :type="row.status === 'active' ? 'success' : 'info'" size="small">
                        {{ row.status === 'active' ? t('supplier.promotions.statusActive') : t('supplier.promotions.statusDisabled') }}
                    </el-tag>
                </template>
            </el-table-column>
            <el-table-column :label="t('user.orders.action')" width="190" fixed="right">
                <template #default="{ row }">
                    <el-button link type="primary" @click="openEdit(row)">{{ t('supplier.promotions.action.edit') }}</el-button>
                    <el-button link type="warning" @click="toggle(row)">{{ t('supplier.promotions.action.toggle') }}</el-button>
                    <el-button link type="danger" @click="remove(row)">{{ t('supplier.promotions.action.delete') }}</el-button>
                </template>
            </el-table-column>
        </el-table>

        <div class="pager">
            <el-pagination v-model:current-page="page" :page-size="pageSize" :total="total" layout="prev, pager, next" @current-change="load" />
        </div>

        <el-dialog v-model="dialogVisible" :title="dialogEditId ? t('supplier.promotions.action.edit') : t('supplier.promotions.dialog.create')" width="460px">
            <el-form :model="dialogForm" label-width="120px">
                <el-form-item v-if="!dialogEditId" :label="t('supplier.promotions.dialog.blindBoxId')" required>
                    <el-input-number v-model="dialogForm.blind_box_id" :min="1" />
                </el-form-item>
                <el-form-item :label="t('supplier.promotions.dialog.originalPrice')" required>
                    <el-input-number v-model="dialogForm.original_price" :min="0" :precision="2" />
                </el-form-item>
                <el-form-item :label="t('supplier.promotions.dialog.promoPrice')" required>
                    <el-input-number v-model="dialogForm.promo_price" :min="0" :precision="2" />
                </el-form-item>
                <el-form-item v-if="!dialogEditId" :label="t('supplier.promotions.dialog.timeRange')" required>
                    <el-date-picker
                        v-model="dialogForm.timeRange"
                        type="datetimerange"
                        value-format="YYYY-MM-DDTHH:mm:ssZ"
                    />
                </el-form-item>
            </el-form>
            <template #footer>
                <el-button @click="dialogVisible = false">{{ t('supplier.promotions.dialog.cancel') }}</el-button>
                <el-button type="primary" :loading="saving" @click="save">{{ t('supplier.promotions.dialog.save') }}</el-button>
            </template>
        </el-dialog>
    </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
    supplierPromotionCreate,
    supplierPromotionDelete,
    supplierPromotionEdit,
    supplierPromotionList,
    supplierPromotionToggle,
    type SupplierPromotion,
} from '/@/api/supplier/promotion'
import { useSupplierInfo } from '/@/stores/supplier/supplierInfo'

const { t } = useI18n()
const router = useRouter()
const supplierInfo = useSupplierInfo()

const items = ref<SupplierPromotion[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 10
const loading = ref(false)
const saving = ref(false)

const dialogVisible = ref(false)
const dialogEditId = ref<number | null>(null)
const dialogForm = reactive({
    blind_box_id: 1,
    original_price: 0,
    promo_price: 0,
    timeRange: [] as string[],
})

function requireLogin(): boolean {
    if (supplierInfo.token) return true
    router.push('/supplier/login')
    return false
}

async function load() {
    if (!requireLogin()) return
    loading.value = true
    try {
        const { data } = await supplierPromotionList({ page: page.value, page_size: pageSize })
        items.value = data.items ?? []
        total.value = data.total
    } catch (err) {
        handle401(err)
    } finally {
        loading.value = false
    }
}

function openCreate() {
    dialogEditId.value = null
    dialogForm.blind_box_id = 1
    dialogForm.original_price = 0
    dialogForm.promo_price = 0
    dialogForm.timeRange = []
    dialogVisible.value = true
}

function openEdit(row: SupplierPromotion) {
    dialogEditId.value = row.id
    dialogForm.original_price = Number(row.original_price)
    dialogForm.promo_price = Number(row.promo_price)
    dialogVisible.value = true
}

async function save() {
    if (dialogForm.promo_price >= dialogForm.original_price) {
        ElMessage.warning(t('supplier.promotions.dialog.promoPriceInvalid'))
        return
    }
    saving.value = true
    try {
        if (dialogEditId.value) {
            await supplierPromotionEdit({
                id: dialogEditId.value,
                original_price: dialogForm.original_price,
                promo_price: dialogForm.promo_price,
            })
        } else {
            if (dialogForm.timeRange.length !== 2) {
                ElMessage.warning(t('supplier.promotions.dialog.timeRange'))
                return
            }
            await supplierPromotionCreate({
                blind_box_id: dialogForm.blind_box_id,
                original_price: dialogForm.original_price,
                promo_price: dialogForm.promo_price,
                start_at: dialogForm.timeRange[0],
                end_at: dialogForm.timeRange[1],
            })
        }
        ElMessage.success('OK')
        dialogVisible.value = false
        await load()
    } catch (err) {
        const anyErr = err as { response?: { status?: number; data?: { code?: string } } }
        if (anyErr?.response?.status === 401) {
            supplierInfo.reset()
            router.push('/supplier/login')
            return
        }
        const code = anyErr?.response?.data?.code ?? ''
        const map: Record<string, string> = {
            'promotion.time_overlap': t('supplier.promotions.timeOverlap'),
            'promotion.invalid_time_window': t('supplier.promotions.invalidTimeWindow'),
            'promotion.invalid_price': t('supplier.promotions.invalidPrice'),
        }
        ElMessage.error(map[code] ?? 'Error')
    } finally {
        saving.value = false
    }
}

async function toggle(row: SupplierPromotion) {
    try {
        await supplierPromotionToggle(row.id)
        await load()
    } catch (err) {
        handle401(err)
    }
}

async function remove(row: SupplierPromotion) {
    try {
        await ElMessageBox.confirm(t('supplier.promotions.confirmDelete'), { type: 'warning' })
    } catch {
        return
    }
    try {
        await supplierPromotionDelete(row.id)
        await load()
    } catch (err) {
        handle401(err)
    }
}

function handle401(err: unknown) {
    const anyErr = err as { response?: { status?: number } }
    if (anyErr?.response?.status === 401) {
        supplierInfo.reset()
        router.push('/supplier/login')
    }
}

onMounted(load)</script>

<style scoped lang="scss">
.page {
    background: #fff;
    border-radius: 16px;
    padding: 24px;
}
.toolbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 16px;
}
.title {
    margin: 0;
    font-size: 20px;
    color: #0f172a;
}
.pager {
    display: flex;
    justify-content: flex-end;
    margin-top: 16px;
}
</style>
