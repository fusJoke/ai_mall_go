<!-- src\views\supplier\bills\index.vue — 供应商账单中心（add-batch-settlement 任务 5.2）。
     状态 tab（全部 + 5 态）+ 账单卡片 + 空态 + 分页；确认/驳回在详情页操作。 -->
<template>
    <div class="page">
        <div class="toolbar">
            <h2 class="title">{{ t('supplier.bills.title') }}</h2>
        </div>

        <el-tabs v-model="activeStatus" @tab-change="onTabChange">
            <el-tab-pane :label="t('supplier.bills.tabAll')" name="all" />
            <el-tab-pane
                v-for="s in BILL_STATUSES"
                :key="s"
                :label="t(`supplier.bills.status.${s}`)"
                :name="s"
            />
        </el-tabs>

        <div v-loading="loading" class="cards">
            <template v-if="items.length">
                <el-card
                    v-for="bill in items"
                    :key="bill.id"
                    class="bill-card"
                    shadow="hover"
                    @click="goDetail(bill.id)"
                >
                    <div class="card-head">
                        <span class="bill-no">#{{ bill.id }}</span>
                        <el-tag :type="statusTagType[bill.status] ?? 'info'" size="small">
                            {{ t(`supplier.bills.status.${bill.status}`) }}
                        </el-tag>
                    </div>
                    <div class="card-body">
                        <div class="amount-row">
                            <div class="amount-cell">
                                <span class="label">{{ t('supplier.bills.columns.totalAmount') }}</span>
                                <span class="value">¥{{ Number(bill.total_amount).toFixed(2) }}</span>
                            </div>
                            <div class="amount-cell">
                                <span class="label">{{ t('supplier.bills.columns.commission') }}</span>
                                <span class="value commission">-¥{{ Number(bill.commission_amount).toFixed(2) }}</span>
                            </div>
                            <div class="amount-cell">
                                <span class="label">{{ t('supplier.bills.columns.payout') }}</span>
                                <span class="value payout">¥{{ Number(bill.payout_amount).toFixed(2) }}</span>
                            </div>
                        </div>
                        <div class="meta-row">
                            <span>{{ t('supplier.bills.columns.blindBox') }}: #{{ bill.blind_box_id }}</span>
                            <span>v{{ bill.version }}</span>
                            <span>{{ fmtTime(bill.created_at) }}</span>
                        </div>
                        <div v-if="bill.status === 'rejected' && bill.reject_reason" class="reject-reason">
                            {{ t('supplier.bills.detail.rejectReason') }}: {{ bill.reject_reason }}
                        </div>
                    </div>
                </el-card>
            </template>
            <el-empty v-else-if="!loading" :description="t('supplier.bills.empty')" />
        </div>

        <div class="pager">
            <el-pagination
                v-model:current-page="page"
                :page-size="pageSize"
                :total="total"
                layout="prev, pager, next"
                @current-change="load"
            />
        </div>
    </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { supplierBillList, type SupplierBillStatus, type SupplierSettlementBill } from '/@/api/supplier/bill'
import { useSupplierInfo } from '/@/stores/supplier/supplierInfo'

const { t } = useI18n()
const router = useRouter()
const supplierInfo = useSupplierInfo()

const BILL_STATUSES: SupplierBillStatus[] = ['generated', 'confirmed', 'rejected', 'failed', 'paid']

const statusTagType: Record<SupplierBillStatus, 'warning' | 'primary' | 'danger' | 'success'> = {
    generated: 'warning',
    confirmed: 'primary',
    rejected: 'danger',
    failed: 'danger',
    paid: 'success',
}

const items = ref<SupplierSettlementBill[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 10
const loading = ref(false)
const activeStatus = ref<'all' | SupplierBillStatus>('all')

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
    loading.value = true
    try {
        const { data } = await supplierBillList({
            status: activeStatus.value === 'all' ? undefined : activeStatus.value,
            page: page.value,
            page_size: pageSize,
        })
        items.value = data.items ?? []
        total.value = data.total
    } catch (err) {
        handle401(err)
    } finally {
        loading.value = false
    }
}

function onTabChange() {
    page.value = 1
    load()
}

function goDetail(id: number) {
    router.push(`/supplier/bills/detail/${id}`)
}

function handle401(err: unknown) {
    const anyErr = err as { response?: { status?: number } }
    if (anyErr?.response?.status === 401) {
        supplierInfo.reset()
        router.push('/supplier/login')
    }
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
    justify-content: space-between;
}
.title {
    margin: 0 0 12px;
    font-size: 20px;
    color: #0f172a;
}
.cards {
    min-height: 200px;
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(340px, 1fr));
    gap: 16px;
    margin-top: 8px;
}
.bill-card {
    cursor: pointer;
    transition: transform 0.15s;

    &:hover {
        transform: translateY(-2px);
    }
}
.card-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 12px;
}
.bill-no {
    font-weight: 700;
    font-size: 16px;
    color: #0f172a;
}
.card-body {
    display: flex;
    flex-direction: column;
    gap: 10px;
}
.amount-row {
    display: flex;
    gap: 8px;
}
.amount-cell {
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 2px;
    background: #f8fafc;
    border-radius: 8px;
    padding: 8px 10px;

    .label {
        font-size: 12px;
        color: #64748b;
    }
    .value {
        font-size: 15px;
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
.meta-row {
    display: flex;
    gap: 16px;
    font-size: 12px;
    color: #94a3b8;
}
.reject-reason {
    font-size: 12px;
    color: #b91c1c;
    background: #fef2f2;
    border-radius: 6px;
    padding: 6px 10px;
}
.pager {
    display: flex;
    justify-content: flex-end;
    margin-top: 16px;
}
</style>
