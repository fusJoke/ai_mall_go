<!-- src\views\admin\settlement\index.vue — admin 结算单管理页（spec 8.9 / 8.10 / 8.11）。

     三个交互入口：
       1. 顶部「预览 / 生成」按钮 → 弹 dialog 输入 (supplier_id + period_start + period_end)
          - 调用 /admin/settlement/preview 拿到 PreviewSettlement
          - 「确认生成」按钮调 /admin/settlement/generate 落库
       2. 列表行「详情」按钮 → 弹 dialog 展示 settlement + items
       3. 列表行「标记已打款」按钮 → 弹确认 dialog → 调 /admin/settlement/mark-paid

     数据来自 /admin/settlement/list（internal/router/admin/mall.go），
     错误码形如 admin.settlement.<action>.<result>（全在 handler/settlement.go 集中映射）。
-->
<template>
    <div class="page">
        <div class="toolbar">
            <el-button type="primary" @click="openPreviewDialog()">
                {{ t('admin.mall.settlement.action.preview') }} /
                {{ t('admin.mall.settlement.action.generate') }}
            </el-button>
        </div>

        <el-table v-loading="loading" :data="items" stripe>
            <el-table-column :label="t('admin.mall.settlement.columns.id')" prop="id" width="100" />
            <el-table-column :label="t('admin.mall.settlement.columns.supplierId')" prop="supplier_id" width="110" />
            <el-table-column :label="t('admin.mall.settlement.columns.periodStart')" min-width="170">
                <template #default="{ row }">{{ new Date(row.period_start).toLocaleString() }}</template>
            </el-table-column>
            <el-table-column :label="t('admin.mall.settlement.columns.periodEnd')" min-width="170">
                <template #default="{ row }">{{ new Date(row.period_end).toLocaleString() }}</template>
            </el-table-column>
            <el-table-column :label="t('admin.mall.settlement.columns.totalAmount')" width="120">
                <template #default="{ row }">¥{{ Number(row.total_amount).toFixed(2) }}</template>
            </el-table-column>
            <el-table-column :label="t('admin.mall.settlement.columns.commissionAmount')" width="120">
                <template #default="{ row }">¥{{ Number(row.commission_amount).toFixed(2) }}</template>
            </el-table-column>
            <el-table-column :label="t('admin.mall.settlement.columns.payoutAmount')" width="120">
                <template #default="{ row }">¥{{ Number(row.payout_amount).toFixed(2) }}</template>
            </el-table-column>
            <el-table-column :label="t('admin.mall.settlement.columns.status')" width="100">
                <template #default="{ row }">
                    <el-tag :type="statusTagType(row.status)" size="small">
                        {{ t('admin.mall.settlement.status.' + row.status) }}
                    </el-tag>
                </template>
            </el-table-column>
            <el-table-column :label="t('admin.mall.settlement.columns.paidAt')" min-width="170">
                <template #default="{ row }">
                    {{ row.paid_at ? new Date(row.paid_at).toLocaleString() : '-' }}
                </template>
            </el-table-column>
            <el-table-column :label="t('admin.common.actions')" width="220" fixed="right">
                <template #default="{ row }">
                    <el-button size="small" link @click="openDetailDialog(row.id)">
                        {{ t('admin.mall.settlement.action.detail') }}
                    </el-button>
                    <el-button
                        size="small"
                        type="primary"
                        link
                        :disabled="row.status === 'paid' || row.status === 'failed'"
                        @click="openMarkPaidDialog(row)"
                    >
                        {{ t('admin.mall.settlement.action.markPaid') }}
                    </el-button>
                </template>
            </el-table-column>
        </el-table>

        <div class="pager">
            <el-pagination
                v-model:current-page="page"
                :page-size="pageSize"
                :total="total"
                layout="prev, pager, next"
                @current-change="load"
            />
        </div>

        <!-- 预览 / 生成 dialog -->
        <el-dialog
            v-model="previewDialog.open"
            :title="t('admin.mall.settlement.dialog.preview')"
            width="560px"
            @closed="resetPreviewForm"
        >
            <el-form :model="previewDialog.form" label-width="140px">
                <el-form-item :label="t('admin.mall.settlement.form.supplierId')">
                    <el-input v-model.number="previewDialog.form.supplier_id" type="number" :min="1" />
                </el-form-item>
                <el-form-item :label="t('admin.mall.settlement.form.periodStart')">
                    <el-input v-model="previewDialog.form.period_start" placeholder="2026-09-01T00:00:00Z" />
                </el-form-item>
                <el-form-item :label="t('admin.mall.settlement.form.periodEnd')">
                    <el-input v-model="previewDialog.form.period_end" placeholder="2026-10-01T00:00:00Z" />
                </el-form-item>
            </el-form>

            <div v-if="previewDialog.preview" class="preview-box">
                <p>
                    <strong>{{ t('admin.mall.settlement.columns.supplierName') }}:</strong>
                    {{ previewDialog.preview.supplier_name }}
                </p>
                <p>
                    <strong>{{ t('admin.mall.settlement.detail.orderCount') }}:</strong>
                    {{ previewDialog.preview.order_count }}
                </p>
                <p>
                    <strong>{{ t('admin.mall.settlement.columns.commissionRate') }}:</strong>
                    {{ (previewDialog.preview.commission_rate * 100).toFixed(2) }}%
                </p>
                <p>
                    <strong>{{ t('admin.mall.settlement.columns.totalAmount') }}:</strong>
                    ¥{{ Number(previewDialog.preview.total_amount).toFixed(2) }}
                </p>
                <p>
                    <strong>{{ t('admin.mall.settlement.columns.commissionAmount') }}:</strong>
                    ¥{{ Number(previewDialog.preview.commission_amount).toFixed(2) }}
                </p>
                <p>
                    <strong>{{ t('admin.mall.settlement.columns.payoutAmount') }}:</strong>
                    ¥{{ Number(previewDialog.preview.payout_amount).toFixed(2) }}
                </p>
            </div>

            <template #footer>
                <el-button @click="previewDialog.open = false">{{ t('admin.common.cancel') }}</el-button>
                <el-button :loading="previewDialog.loading" @click="runPreview">预览</el-button>
                <el-button
                    type="primary"
                    :loading="previewDialog.generating"
                    :disabled="!previewDialog.preview || previewDialog.preview.order_count === 0"
                    @click="runGenerate"
                >
                    {{ t('admin.mall.settlement.action.generate') }}
                </el-button>
            </template>
        </el-dialog>

        <!-- 详情 dialog -->
        <el-dialog
            v-model="detailDialog.open"
            :title="t('admin.mall.settlement.dialog.generate') + ' #' + (detailDialog.settlement?.id ?? '')"
            width="800px"
        >
            <div v-if="detailDialog.settlement" class="detail-summary">
                <p>
                    <strong>{{ t('admin.mall.settlement.columns.supplierId') }}:</strong>
                    {{ detailDialog.settlement.supplier_id }}
                </p>
                <p>
                    <strong>{{ t('admin.mall.settlement.columns.periodStart') }}:</strong>
                    {{ new Date(detailDialog.settlement.period_start).toLocaleString() }}
                    ~
                    {{ new Date(detailDialog.settlement.period_end).toLocaleString() }}
                </p>
                <p>
                    <strong>{{ t('admin.mall.settlement.columns.totalAmount') }}:</strong>
                    ¥{{ Number(detailDialog.settlement.total_amount).toFixed(2) }}
                </p>
                <p>
                    <strong>{{ t('admin.mall.settlement.columns.commissionAmount') }}:</strong>
                    ¥{{ Number(detailDialog.settlement.commission_amount).toFixed(2) }}
                    ({{ (detailDialog.settlement.commission_rate * 100).toFixed(2) }}%)
                </p>
                <p>
                    <strong>{{ t('admin.mall.settlement.columns.payoutAmount') }}:</strong>
                    ¥{{ Number(detailDialog.settlement.payout_amount).toFixed(2) }}
                </p>
                <p>
                    <strong>{{ t('admin.mall.settlement.columns.status') }}:</strong>
                    <el-tag :type="statusTagType(detailDialog.settlement.status)" size="small">
                        {{ t('admin.mall.settlement.status.' + detailDialog.settlement.status) }}
                    </el-tag>
                </p>
            </div>

            <el-table v-if="detailDialog.items.length" :data="detailDialog.items" stripe>
                <el-table-column :label="t('admin.mall.settlement.detail.orders') + ' ID'" prop="draw_order_id" width="120" />
                <el-table-column label="amount" width="120">
                    <template #default="{ row }">¥{{ Number(row.amount).toFixed(2) }}</template>
                </el-table-column>
                <el-table-column label="commission" width="120">
                    <template #default="{ row }">¥{{ Number(row.commission_amount).toFixed(2) }}</template>
                </el-table-column>
                <el-table-column label="payout" width="120">
                    <template #default="{ row }">¥{{ Number(row.payout_amount).toFixed(2) }}</template>
                </el-table-column>
            </el-table>
        </el-dialog>

        <!-- 标记已打款 dialog -->
        <el-dialog
            v-model="markPaidDialog.open"
            :title="t('admin.mall.settlement.dialog.markPaid')"
            width="420px"
        >
            <p v-if="markPaidDialog.target">
                {{ t('admin.mall.settlement.message.confirmMarkPaid') }}
            </p>
            <p v-if="markPaidDialog.target" class="mark-paid-target">
                #{{ markPaidDialog.target.id }}
                ·
                ¥{{ Number(markPaidDialog.target.payout_amount).toFixed(2) }}
            </p>
            <template #footer>
                <el-button @click="markPaidDialog.open = false">{{ t('admin.common.cancel') }}</el-button>
                <el-button
                    type="primary"
                    :loading="markPaidDialog.loading"
                    @click="runMarkPaid"
                >
                    {{ t('admin.mall.settlement.action.markPaid') }}
                </el-button>
            </template>
        </el-dialog>
    </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useI18n } from 'vue-i18n'

import {
    mallSettlementDetail,
    mallSettlementGenerate,
    mallSettlementList,
    mallSettlementMarkPaid,
    mallSettlementPreview,
    type MallSettlement,
    type MallSettlementItem,
    type MallSettlementPreview,
} from '/@/api/admin/mall'

const { t } = useI18n()

// ---------- list ----------
const items = ref<MallSettlement[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 20
const loading = ref(false)

async function load() {
    loading.value = true
    try {
        const { data } = await mallSettlementList({ page: page.value, page_size: pageSize })
        items.value = data.items ?? []
        total.value = data.total
    } finally {
        loading.value = false
    }
}

function statusTagType(s: MallSettlement['status']) {
    switch (s) {
        case 'paid':
            return 'success'
        case 'pending':
            return 'warning'
        case 'processing':
            return 'primary'
        case 'failed':
            return 'danger'
    }
}

// ---------- preview dialog ----------
const previewDialog = reactive({
    open: false,
    loading: false,
    generating: false,
    form: { supplier_id: 0, period_start: '', period_end: '' },
    preview: null as MallSettlementPreview | null,
})

function openPreviewDialog() {
    previewDialog.open = true
}

function resetPreviewForm() {
    previewDialog.form = { supplier_id: 0, period_start: '', period_end: '' }
    previewDialog.preview = null
}

async function runPreview() {
    previewDialog.loading = true
    previewDialog.preview = null
    try {
        const { data } = await mallSettlementPreview({ ...previewDialog.form })
        previewDialog.preview = data
        if (data.order_count === 0) {
            ElMessage.warning(t('admin.mall.settlement.message.previewEmpty'))
        }
    } finally {
        previewDialog.loading = false
    }
}

async function runGenerate() {
    if (!previewDialog.preview) return
    try {
        await ElMessageBox.confirm(t('admin.mall.settlement.message.confirmMarkPaid'), '', {
            type: 'warning',
        }).catch(() => null)
    } catch { /* fallthrough */ }

    previewDialog.generating = true
    try {
        const { data } = await mallSettlementGenerate({ ...previewDialog.form })
        ElMessage.success(t('admin.mall.settlement.message.generateOk'))
        previewDialog.open = false
        await load()
        // 跳到刚生成的 settlement 详情
        if (data?.settlement?.id) {
            await openDetailDialog(data.settlement.id)
        }
    } finally {
        previewDialog.generating = false
    }
}

// ---------- detail dialog ----------
const detailDialog = reactive({
    open: false,
    settlement: null as MallSettlement | null,
    items: [] as MallSettlementItem[],
})

async function openDetailDialog(id: number) {
    detailDialog.open = true
    try {
        const { data } = await mallSettlementDetail(id)
        detailDialog.settlement = data.settlement
        detailDialog.items = data.items ?? []
    } catch {
        detailDialog.open = false
    }
}

// ---------- mark-paid dialog ----------
const markPaidDialog = reactive({
    open: false,
    loading: false,
    target: null as MallSettlement | null,
})

function openMarkPaidDialog(row: MallSettlement) {
    markPaidDialog.target = row
    markPaidDialog.open = true
}

async function runMarkPaid() {
    if (!markPaidDialog.target) return
    markPaidDialog.loading = true
    try {
        await mallSettlementMarkPaid(markPaidDialog.target.id)
        ElMessage.success(t('admin.mall.settlement.message.markPaidOk'))
        markPaidDialog.open = false
        await load()
    } finally {
        markPaidDialog.loading = false
    }
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
    margin-bottom: 16px;
}

.pager {
    display: flex;
    justify-content: flex-end;
    margin-top: 16px;
}

.preview-box {
    background: #f7f8fa;
    border-radius: 6px;
    padding: 12px 16px;
    margin-top: 8px;

    p {
        margin: 4px 0;
    }
}

.detail-summary {
    background: #f7f8fa;
    border-radius: 6px;
    padding: 12px 16px;
    margin-bottom: 16px;

    p {
        margin: 4px 0;
    }
}

.mark-paid-target {
    font-weight: 600;
    color: #d97706;
}
</style>