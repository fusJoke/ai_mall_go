<!-- src\views\admin\bills\index.vue — admin 批次账单管理页（add-batch-settlement 任务 5.4）。

     批次账单 = 「1 商品 = 1 账单」，商品售罄时由抽卡事务自动生成（见 tasks.md 3.2）。
     admin 在此做两件事：
       1. 核对 rejected 账单（供应商驳回后）→ 重核弹窗改金额四件套，提交后
          账单回到 generated、version+1、清空 reject_reason。
       2. 对 confirmed 账单打款 → 二次确认展示扣减额 → 事务内
          bill→paid + supplier.balance -= payout + 商品置 settled。

     数据来自 /admin/bill/{list,detail,regenerate,mark-paid}
     （internal/router/admin/mall.go），错误码 admin.bill.<action>.<result>。
-->
<template>
    <div class="page">
        <div class="toolbar">
            <el-tabs v-model="activeStatus" class="status-tabs" @tab-change="onTabChange">
                <el-tab-pane :label="t('admin.mall.bill.tabAll')" name="all" />
                <el-tab-pane :label="t('admin.mall.bill.status.generated')" name="generated" />
                <el-tab-pane :label="t('admin.mall.bill.status.confirmed')" name="confirmed" />
                <el-tab-pane :label="t('admin.mall.bill.status.paid')" name="paid" />
                <el-tab-pane :label="t('admin.mall.bill.status.rejected')" name="rejected" />
                <el-tab-pane :label="t('admin.mall.bill.status.failed')" name="failed" />
            </el-tabs>
            <div class="filter">
                <el-input
                    v-model="supplierIdInput"
                    class="supplier-filter"
                    :placeholder="t('admin.mall.bill.supplierFilter')"
                    clearable
                    @keyup.enter="onFilterSupplier"
                    @clear="onFilterSupplier"
                />
                <el-button @click="onFilterSupplier">{{ t('admin.mall.common.search') }}</el-button>
            </div>
        </div>

        <el-table v-loading="loading" :data="items" stripe>
            <el-table-column :label="t('admin.mall.bill.columns.id')" prop="id" width="90" />
            <el-table-column :label="t('admin.mall.bill.columns.blindBoxId')" prop="blind_box_id" width="110" />
            <el-table-column :label="t('admin.mall.bill.columns.supplierId')" prop="supplier_id" width="110" />
            <el-table-column :label="t('admin.mall.bill.columns.totalAmount')" width="120">
                <template #default="{ row }">¥{{ money(row.total_amount) }}</template>
            </el-table-column>
            <el-table-column :label="t('admin.mall.bill.columns.commissionRate')" width="100">
                <template #default="{ row }">{{ (Number(row.commission_rate) * 100).toFixed(2) }}%</template>
            </el-table-column>
            <el-table-column :label="t('admin.mall.bill.columns.commissionAmount')" width="120">
                <template #default="{ row }">¥{{ money(row.commission_amount) }}</template>
            </el-table-column>
            <el-table-column :label="t('admin.mall.bill.columns.payoutAmount')" width="120">
                <template #default="{ row }">
                    <span class="payout">¥{{ money(row.payout_amount) }}</span>
                </template>
            </el-table-column>
            <el-table-column :label="t('admin.mall.bill.columns.status')" width="110">
                <template #default="{ row }">
                    <el-tag :type="statusTagType(lcOf(row))" size="small">
                        {{ t('admin.mall.bill.status.' + lcOf(row)) }}
                    </el-tag>
                </template>
            </el-table-column>
            <el-table-column :label="t('admin.mall.bill.columns.version')" prop="version" width="80" />
            <el-table-column :label="t('admin.mall.bill.columns.createdAt')" min-width="165">
                <template #default="{ row }">{{ fmtTime(row.created_at) }}</template>
            </el-table-column>
            <el-table-column :label="t('admin.mall.common.actions')" width="200" fixed="right">
                <template #default="{ row }">
                    <el-button size="small" link @click="openDetail(row.id)">
                        {{ t('admin.mall.bill.action.detail') }}
                    </el-button>
                    <el-button
                        v-if="lcOf(row) === 'rejected'"
                        size="small"
                        type="warning"
                        link
                        @click="openRegenerate(row)"
                    >
                        {{ t('admin.mall.bill.action.regenerate') }}
                    </el-button>
                    <el-button
                        v-if="lcOf(row) === 'confirmed'"
                        size="small"
                        type="primary"
                        link
                        @click="openMarkPaid(row)"
                    >
                        {{ t('admin.mall.bill.action.markPaid') }}
                    </el-button>
                </template>
            </el-table-column>
        </el-table>

        <el-empty v-if="!loading && !items.length" :description="t('admin.mall.bill.empty')" />

        <div class="pager">
            <el-pagination
                v-model:current-page="page"
                :page-size="pageSize"
                :total="total"
                layout="prev, pager, next"
                @current-change="load"
            />
        </div>

        <!-- 详情弹窗 -->
        <el-dialog
            v-model="detailDialog.open"
            :title="t('admin.mall.bill.dialog.detail') + ' #' + (detailDialog.bill?.id ?? '')"
            width="820px"
        >
            <div v-if="detailDialog.bill" class="summary">
                <p>
                    <strong>{{ t('admin.mall.bill.columns.blindBoxId') }}:</strong>
                    {{ detailDialog.bill.blind_box_id }}
                </p>
                <p>
                    <strong>{{ t('admin.mall.bill.columns.supplierId') }}:</strong>
                    {{ detailDialog.bill.supplier_id }}
                </p>
                <p>
                    <strong>{{ t('admin.mall.bill.columns.totalAmount') }}:</strong>
                    ¥{{ money(detailDialog.bill.total_amount) }}
                    ·
                    <strong>{{ t('admin.mall.bill.columns.commissionAmount') }}:</strong>
                    ¥{{ money(detailDialog.bill.commission_amount) }}
                    ({{ (Number(detailDialog.bill.commission_rate) * 100).toFixed(2) }}%)
                    ·
                    <strong>{{ t('admin.mall.bill.columns.payoutAmount') }}:</strong>
                    <span class="payout">¥{{ money(detailDialog.bill.payout_amount) }}</span>
                </p>
                <p>
                    <strong>{{ t('admin.mall.bill.columns.status') }}:</strong>
                    <el-tag :type="statusTagType(detailDialog.bill.status)" size="small">
                        {{ t('admin.mall.bill.status.' + detailDialog.bill.status) }}
                    </el-tag>
                    <span class="muted">v{{ detailDialog.bill.version }}</span>
                </p>
                <p v-if="detailDialog.bill.reject_reason">
                    <strong>{{ t('admin.mall.bill.columns.rejectReason') }}:</strong>
                    <span class="danger-text">{{ detailDialog.bill.reject_reason }}</span>
                </p>
                <p v-if="detailDialog.bill.remark">
                    <strong>{{ t('admin.mall.bill.columns.remark') }}:</strong>
                    {{ detailDialog.bill.remark }}
                </p>
                <p>
                    <strong>{{ t('admin.mall.bill.columns.paidAt') }}:</strong>
                    {{ detailDialog.bill.paid_at ? fmtTime(detailDialog.bill.paid_at) : '-' }}
                </p>
            </div>

            <h4 class="items-title">
                {{ t('admin.mall.bill.detail.items') }}
                <span class="muted">({{ detailDialog.items.length }})</span>
            </h4>
            <el-table v-if="detailDialog.items.length" :data="detailDialog.items" stripe max-height="320">
                <el-table-column :label="t('admin.mall.bill.detail.orderId')" prop="draw_order_id" width="120" />
                <el-table-column label="amount" width="120">
                    <template #default="{ row }">¥{{ money(row.amount) }}</template>
                </el-table-column>
                <el-table-column label="commission" width="120">
                    <template #default="{ row }">¥{{ money(row.commission_amount) }}</template>
                </el-table-column>
                <el-table-column label="payout" width="120">
                    <template #default="{ row }">¥{{ money(row.payout_amount) }}</template>
                </el-table-column>
            </el-table>
            <el-empty v-else :description="t('admin.mall.bill.detail.noItems')" />
        </el-dialog>

        <!-- 重核弹窗：金额四件套整体可改 -->
        <el-dialog
            v-model="regenDialog.open"
            :title="t('admin.mall.bill.dialog.regenerate') + ' #' + (regenDialog.id ?? '')"
            width="560px"
        >
            <el-alert
                type="warning"
                :closable="false"
                show-icon
                class="regen-alert"
                :title="t('admin.mall.bill.dialog.regenerateHint')"
            />
            <el-form :model="regenDialog.form" label-width="150px" class="regen-form">
                <el-form-item :label="t('admin.mall.bill.columns.totalAmount')">
                    <el-input-number
                        v-model="regenDialog.form.total_amount"
                        :min="0"
                        :precision="2"
                        :step="1"
                        class="num"
                        @change="recalcFromRate"
                    />
                </el-form-item>
                <el-form-item :label="t('admin.mall.bill.columns.commissionRate')">
                    <el-input-number
                        v-model="regenDialog.form.commission_rate"
                        :min="0"
                        :max="1"
                        :precision="4"
                        :step="0.01"
                        class="num"
                        @change="recalcFromRate"
                    />
                    <span class="muted">0 ~ 1，如 0.1 = 10%</span>
                </el-form-item>
                <el-form-item :label="t('admin.mall.bill.columns.commissionAmount')">
                    <el-input-number
                        v-model="regenDialog.form.commission_amount"
                        :min="0"
                        :precision="2"
                        :step="1"
                        class="num"
                    />
                </el-form-item>
                <el-form-item :label="t('admin.mall.bill.columns.payoutAmount')">
                    <el-input-number
                        v-model="regenDialog.form.payout_amount"
                        :min="0"
                        :precision="2"
                        :step="1"
                        class="num"
                    />
                </el-form-item>
                <el-form-item :label="t('admin.mall.bill.columns.remark')">
                    <el-input
                        v-model="regenDialog.form.remark"
                        type="textarea"
                        :rows="2"
                        :placeholder="t('admin.mall.bill.dialog.remarkPlaceholder')"
                    />
                </el-form-item>
            </el-form>
            <template #footer>
                <el-button @click="regenDialog.open = false">{{ t('admin.common.cancel') }}</el-button>
                <el-button type="primary" :loading="regenDialog.loading" @click="submitRegenerate">
                    {{ t('admin.mall.bill.action.regenerate') }}
                </el-button>
            </template>
        </el-dialog>

        <!-- 打款二次确认：显式展示将扣减的金额 -->
        <el-dialog
            v-model="markPaidDialog.open"
            :title="t('admin.mall.bill.dialog.markPaid')"
            width="440px"
        >
            <template v-if="markPaidDialog.bill">
                <p>{{ t('admin.mall.bill.dialog.markPaidConfirm') }}</p>
                <div class="deduct-box">
                    <div class="deduct-row">
                        <span>{{ t('admin.mall.bill.dialog.bill') }}</span>
                        <span>#{{ markPaidDialog.bill.id }}</span>
                    </div>
                    <div class="deduct-row">
                        <span>{{ t('admin.mall.bill.dialog.supplier') }}</span>
                        <span>{{ markPaidDialog.bill.supplier_id }}</span>
                    </div>
                    <div class="deduct-row total">
                        <span>{{ t('admin.mall.bill.dialog.deduct') }}</span>
                        <span class="payout">-¥{{ money(markPaidDialog.bill.payout_amount) }}</span>
                    </div>
                </div>
                <p class="muted">{{ t('admin.mall.bill.dialog.markPaidTail') }}</p>
            </template>
            <template #footer>
                <el-button @click="markPaidDialog.open = false">{{ t('admin.common.cancel') }}</el-button>
                <el-button type="primary" :loading="markPaidDialog.loading" @click="submitMarkPaid">
                    {{ t('admin.mall.bill.action.markPaid') }}
                </el-button>
            </template>
        </el-dialog>
    </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useI18n } from 'vue-i18n'

import {
    adminBillDetail,
    adminBillList,
    adminBillMarkPaid,
    adminBillRegenerate,
    type AdminBillStatus,
    type AdminSettlementBill,
    type AdminSettlementBillItem,
} from '/@/api/admin/bill'

const { t } = useI18n()

// ---------- list ----------
const items = ref<AdminSettlementBill[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 20
const loading = ref(false)
const activeStatus = ref<string>('all')
const supplierIdInput = ref('')

/** el-table 插槽的 row 是 any，直接 row.status 会触发 TS7053（与 supplier 侧同款处理）。 */
function lcOf(row: AdminSettlementBill): AdminBillStatus {
    return row.status
}

async function load() {
    loading.value = true
    try {
        const { data } = await adminBillList({
            status: activeStatus.value === 'all' ? undefined : activeStatus.value,
            supplier_id: supplierIdInput.value ? Number(supplierIdInput.value) : undefined,
            page: page.value,
            page_size: pageSize,
        })
        items.value = data.items ?? []
        total.value = data.total
    } finally {
        loading.value = false
    }
}

function onTabChange() {
    page.value = 1
    load()
}

function onFilterSupplier() {
    page.value = 1
    load()
}

function statusTagType(s: AdminBillStatus) {
    switch (s) {
        case 'paid':
            return 'success'
        case 'confirmed':
            return 'primary'
        case 'generated':
            return 'warning'
        case 'rejected':
        case 'failed':
            return 'danger'
    }
}

function money(v: number | string) {
    return Number(v ?? 0).toFixed(2)
}

function fmtTime(v: string | null) {
    return v ? new Date(v).toLocaleString() : '-'
}

// ---------- detail ----------
const detailDialog = reactive({
    open: false,
    bill: null as AdminSettlementBill | null,
    items: [] as AdminSettlementBillItem[],
})

async function openDetail(id: number) {
    detailDialog.open = true
    try {
        const { data } = await adminBillDetail(id)
        detailDialog.bill = data.bill
        detailDialog.items = data.items ?? []
    } catch {
        detailDialog.open = false
    }
}

// ---------- regenerate ----------
const regenDialog = reactive({
    open: false,
    loading: false,
    id: 0 as number,
    form: { total_amount: 0, commission_rate: 0, commission_amount: 0, payout_amount: 0, remark: '' },
})

function openRegenerate(row: AdminSettlementBill) {
    regenDialog.id = row.id
    regenDialog.form = {
        total_amount: Number(row.total_amount),
        commission_rate: Number(row.commission_rate),
        commission_amount: Number(row.commission_amount),
        payout_amount: Number(row.payout_amount),
        remark: '',
    }
    regenDialog.open = true
}

/** 改总额或费率时联动重算手续费 / 应打款，减少手算错误（仍可手改）。 */
function recalcFromRate() {
    const f = regenDialog.form
    const total = Number(f.total_amount) || 0
    const rate = Number(f.commission_rate) || 0
    f.commission_amount = Number((total * rate).toFixed(2))
    f.payout_amount = Number((total - f.commission_amount).toFixed(2))
}

async function submitRegenerate() {
    regenDialog.loading = true
    try {
        await adminBillRegenerate({
            id: regenDialog.id,
            total_amount: regenDialog.form.total_amount,
            commission_rate: regenDialog.form.commission_rate,
            commission_amount: regenDialog.form.commission_amount,
            payout_amount: regenDialog.form.payout_amount,
            remark: regenDialog.form.remark.trim() || undefined,
        })
        ElMessage.success(t('admin.mall.bill.regenerated'))
        regenDialog.open = false
        await load()
    } catch (err) {
        showActionError(err)
    } finally {
        regenDialog.loading = false
    }
}

// ---------- mark paid ----------
const markPaidDialog = reactive({
    open: false,
    loading: false,
    bill: null as AdminSettlementBill | null,
})

function openMarkPaid(row: AdminSettlementBill) {
    markPaidDialog.bill = row
    markPaidDialog.open = true
}

async function submitMarkPaid() {
    if (!markPaidDialog.bill) return
    markPaidDialog.loading = true
    try {
        await adminBillMarkPaid(markPaidDialog.bill.id)
        ElMessage.success(t('admin.mall.bill.markPaidOk'))
        markPaidDialog.open = false
        await load()
    } catch (err) {
        showActionError(err)
    } finally {
        markPaidDialog.loading = false
    }
}

// ---------- error ----------
/** 操作类错误统一提示；409 三码文案不同（后端约定，见 handler/admin/bill.go）。 */
function showActionError(err: unknown) {
    const code = errCode(err).replaceAll('.', '_')
    const key =
        {
            admin_bill_regenerate_not_found: 'admin.mall.bill.notFound',
            admin_bill_regenerate_invalid_transition: 'admin.mall.bill.invalidTransition',
            admin_bill_regenerate_status_conflict: 'admin.mall.bill.statusConflict',
            admin_bill_mark_paid_not_found: 'admin.mall.bill.notFound',
            admin_bill_mark_paid_invalid_transition: 'admin.mall.bill.invalidTransition',
            admin_bill_mark_paid_status_conflict: 'admin.mall.bill.statusConflict',
            admin_bill_mark_paid_box_lifecycle_conflict: 'admin.mall.bill.boxLifecycleConflict',
            admin_bill_mark_paid_insufficient_balance: 'admin.mall.bill.insufficientBalance',
        }[code] ?? 'admin.mall.bill.failed'
    ElMessage.error(t(key))
}

function errCode(err: unknown): string {
    return (err as { response?: { data?: { code?: string } } })?.response?.data?.code ?? ''
}

onMounted(load)
</script>

<style scoped lang="scss">
.page {
    background: #fff;
    border-radius: 8px;
    padding: 20px;
}

.toolbar {
    display: flex;
    align-items: center;
    gap: 16px;
    margin-bottom: 8px;
}
.status-tabs {
    flex: 1;

    :deep(.el-tabs__header) {
        margin-bottom: 0;
    }
}
.filter {
    display: flex;
    gap: 8px;
}
.supplier-filter {
    width: 180px;
}

.payout {
    font-weight: 600;
    color: #d97706;
}
.muted {
    color: #94a3b8;
    font-size: 12px;
    margin-left: 6px;
}
.danger-text {
    color: #dc2626;
}

.pager {
    display: flex;
    justify-content: flex-end;
    margin-top: 16px;
}

.summary {
    background: #f8fafc;
    border-radius: 6px;
    padding: 12px 16px;

    p {
        margin: 4px 0;
    }
}

.items-title {
    margin: 16px 0 8px;
    font-size: 14px;
    color: #0f172a;
}

.regen-alert {
    margin-bottom: 12px;
}
.regen-form {
    :deep(.el-form-item) {
        margin-bottom: 14px;
    }
}
.num {
    width: 180px;
}

.deduct-box {
    background: #f8fafc;
    border-radius: 6px;
    padding: 12px 16px;
    margin: 12px 0 8px;
}
.deduct-row {
    display: flex;
    justify-content: space-between;
    padding: 3px 0;
    color: #475569;

    &.total {
        border-top: 1px solid #e2e8f0;
        margin-top: 6px;
        padding-top: 8px;
        font-weight: 600;
        color: #0f172a;
    }
}
</style>
