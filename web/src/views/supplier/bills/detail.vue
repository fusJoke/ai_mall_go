<!-- src\views\supplier\bills\detail.vue — 账单详情（add-batch-settlement 任务 5.2）。
     金额三联（GMV/手续费/应打款）+ 账单信息 + 订单明细表 + 确认/驳回操作。
     409 双码区分：invalid_transition = 状态机拒绝；status_conflict = 并发抢先。 -->
<template>
    <div class="page" v-loading="loading">
        <div class="toolbar">
            <el-button link type="primary" @click="router.push('/supplier/bills')">
                ← {{ t('supplier.bills.detail.back') }}
            </el-button>
            <h2 class="title">{{ t('supplier.bills.detail.title') }} #{{ billId }}</h2>
            <span />
        </div>

        <template v-if="bill">
            <div class="stats">
                <div class="stat">
                    <span class="label">{{ t('supplier.bills.detail.gmv') }}</span>
                    <span class="value">¥{{ Number(bill.total_amount).toFixed(2) }}</span>
                </div>
                <div class="stat">
                    <span class="label">
                        {{ t('supplier.bills.detail.commission') }}（{{ commissionPercent }}%）
                    </span>
                    <span class="value commission">-¥{{ Number(bill.commission_amount).toFixed(2) }}</span>
                </div>
                <div class="stat payout-stat">
                    <span class="label">{{ t('supplier.bills.detail.payout') }}</span>
                    <span class="value payout">¥{{ Number(bill.payout_amount).toFixed(2) }}</span>
                </div>
            </div>

            <el-descriptions :column="3" border class="info">
                <el-descriptions-item :label="t('supplier.bills.columns.status')">
                    <el-tag :type="statusTagType[bill.status] ?? 'info'" size="small">
                        {{ t(`supplier.bills.status.${bill.status}`) }}
                    </el-tag>
                </el-descriptions-item>
                <el-descriptions-item :label="t('supplier.bills.detail.blindBox')">
                    #{{ bill.blind_box_id }}
                </el-descriptions-item>
                <el-descriptions-item :label="t('supplier.bills.detail.version')">
                    v{{ bill.version }}
                </el-descriptions-item>
                <el-descriptions-item :label="t('supplier.bills.detail.createdAt')">
                    {{ fmtTime(bill.created_at) }}
                </el-descriptions-item>
                <el-descriptions-item :label="t('supplier.bills.detail.confirmedAt')">
                    {{ fmtTime(bill.confirmed_at) }}
                </el-descriptions-item>
                <el-descriptions-item :label="t('supplier.bills.detail.paidAt')">
                    {{ fmtTime(bill.paid_at) }}
                </el-descriptions-item>
                <el-descriptions-item
                    v-if="bill.reject_reason"
                    :label="t('supplier.bills.detail.rejectReason')"
                    :span="2"
                >
                    <span class="reject-text">{{ bill.reject_reason }}</span>
                </el-descriptions-item>
                <el-descriptions-item v-if="bill.remark" :label="t('supplier.bills.detail.remark')" :span="2">
                    {{ bill.remark }}
                </el-descriptions-item>
            </el-descriptions>

            <div class="section-head">
                <h3 class="section-title">{{ t('supplier.bills.detail.items') }}</h3>
                <div v-if="bill.status === 'generated'" class="actions">
                    <el-button type="primary" :loading="acting" @click="onConfirm">
                        {{ t('supplier.bills.action.confirm') }}
                    </el-button>
                    <el-button type="danger" plain :loading="acting" @click="rejectVisible = true">
                        {{ t('supplier.bills.action.reject') }}
                    </el-button>
                </div>
            </div>

            <el-table :data="items" stripe>
                <el-table-column prop="draw_order_id" :label="t('supplier.bills.detail.orderNo')" width="120">
                    <template #default="{ row }">#{{ row.draw_order_id }}</template>
                </el-table-column>
                <el-table-column :label="t('supplier.bills.detail.orderAmount')" width="140">
                    <template #default="{ row }">¥{{ Number(row.amount).toFixed(2) }}</template>
                </el-table-column>
                <el-table-column :label="t('supplier.bills.detail.orderCommission')" width="140">
                    <template #default="{ row }">¥{{ Number(row.commission_amount).toFixed(2) }}</template>
                </el-table-column>
                <el-table-column :label="t('supplier.bills.detail.orderPayout')" width="140">
                    <template #default="{ row }">¥{{ Number(row.payout_amount).toFixed(2) }}</template>
                </el-table-column>
                <el-table-column :label="t('supplier.bills.detail.orderTime')">
                    <template #default="{ row }">{{ fmtTime(row.created_at) }}</template>
                </el-table-column>
            </el-table>
            <el-empty v-if="!loading && !items.length" :description="t('supplier.bills.detail.itemsEmpty')" />
        </template>

        <el-dialog
            v-model="rejectVisible"
            :title="t('supplier.bills.action.rejectTitle')"
            width="480px"
            :close-on-click-modal="false"
        >
            <el-input
                v-model="rejectReason"
                type="textarea"
                :rows="3"
                maxlength="500"
                show-word-limit
                :placeholder="t('supplier.bills.action.rejectPlaceholder')"
            />
            <template #footer>
                <el-button @click="rejectVisible = false">
                    {{ t('supplier.products.dialog.cancel') }}
                </el-button>
                <el-button type="danger" :loading="acting" @click="onReject">
                    {{ t('supplier.bills.action.reject') }}
                </el-button>
            </template>
        </el-dialog>
    </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
    supplierBillConfirm,
    supplierBillDetail,
    supplierBillReject,
    type SupplierBillStatus,
    type SupplierSettlementBill,
    type SupplierSettlementBillItem,
} from '/@/api/supplier/bill'
import { useSupplierInfo } from '/@/stores/supplier/supplierInfo'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const supplierInfo = useSupplierInfo()

const billId = Number(route.params.id)

const statusTagType: Record<SupplierBillStatus, 'warning' | 'primary' | 'danger' | 'success'> = {
    generated: 'warning',
    confirmed: 'primary',
    rejected: 'danger',
    failed: 'danger',
    paid: 'success',
}

const bill = ref<SupplierSettlementBill | null>(null)
const items = ref<SupplierSettlementBillItem[]>([])
const loading = ref(false)
const acting = ref(false)
const rejectVisible = ref(false)
const rejectReason = ref('')

const commissionPercent = computed(() =>
    bill.value ? (Number(bill.value.commission_rate) * 100).toFixed(2) : '0.00',
)

function requireLogin(): boolean {
    if (supplierInfo.token) return true
    router.push('/supplier/login')
    return false
}

function fmtTime(v: string | null): string {
    if (!v) return '—'
    return new Date(v).toLocaleString()
}

async function load() {
    if (!requireLogin()) return
    if (!Number.isFinite(billId) || billId <= 0) {
        router.replace('/404')
        return
    }
    loading.value = true
    try {
        const { data } = await supplierBillDetail(billId)
        bill.value = data.bill
        items.value = data.items ?? []
    } catch (err) {
        if (is401(err)) {
            supplierInfo.reset()
            router.push('/supplier/login')
            return
        }
        const code = errCode(err)
        ElMessage.error(code === 'bill.not_found' ? t('supplier.bills.notFound') : t('supplier.bills.loadFailed'))
    } finally {
        loading.value = false
    }
}

async function onConfirm() {
    try {
        await ElMessageBox.confirm(t('supplier.bills.action.confirmAsk'), t('supplier.bills.action.confirm'), {
            type: 'warning',
        })
    } catch {
        return // 用户取消
    }
    acting.value = true
    try {
        await supplierBillConfirm(billId)
        ElMessage.success(t('supplier.bills.confirmed'))
        await load()
    } catch (err) {
        showActionError(err)
    } finally {
        acting.value = false
    }
}

async function onReject() {
    const reason = rejectReason.value.trim()
    if (!reason) {
        ElMessage.warning(t('supplier.bills.action.rejectRequired'))
        return
    }
    acting.value = true
    try {
        await supplierBillReject(billId, reason)
        rejectVisible.value = false
        rejectReason.value = ''
        ElMessage.success(t('supplier.bills.rejected'))
        await load()
    } catch (err) {
        showActionError(err)
    } finally {
        acting.value = false
    }
}

/** 操作类错误统一提示；409 双码文案不同（后端约定）。 */
function showActionError(err: unknown) {
    if (is401(err)) {
        supplierInfo.reset()
        router.push('/supplier/login')
        return
    }
    const code = errCode(err)
    const key =
        {
            bill_not_found: 'supplier.bills.notFound',
            bill_forbidden: 'supplier.bills.forbidden',
            bill_invalid_transition: 'supplier.bills.invalidTransition',
            bill_status_conflict: 'supplier.bills.statusConflict',
            bill_reject_reason_required: 'supplier.bills.action.rejectRequired',
        }[code.replaceAll('.', '_')] ?? 'supplier.bills.failed'
    ElMessage.error(t(key))
}

function is401(err: unknown): boolean {
    return (err as { response?: { status?: number } })?.response?.status === 401
}

function errCode(err: unknown): string {
    return (err as { response?: { data?: { code?: string } } })?.response?.data?.code ?? ''
}

onMounted(load)
</script>

<style scoped lang="scss">
.page {
    background: #fff;
    border-radius: 16px;
    padding: 24px;
}
.toolbar {
    display: flex;
    align-items: center;
    gap: 12px;

    > :last-child {
        flex: 1;
    }
}
.title {
    margin: 0;
    font-size: 20px;
    color: #0f172a;
}
.stats {
    display: flex;
    gap: 16px;
    margin: 16px 0;
}
.stat {
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 4px;
    background: #f8fafc;
    border-radius: 12px;
    padding: 16px 20px;

    .label {
        font-size: 13px;
        color: #64748b;
    }
    .value {
        font-size: 24px;
        font-weight: 700;
        color: #0f172a;
    }
    .commission {
        color: #ef4444;
    }
    .payout {
        color: #16a34a;
    }
}
.payout-stat {
    background: #f0fdf4;
}
.info {
    margin-bottom: 20px;
}
.reject-text {
    color: #b91c1c;
}
.section-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 12px;
}
.section-title {
    margin: 0;
    font-size: 16px;
    color: #0f172a;
}
.actions {
    display: flex;
    gap: 8px;
}
</style>
