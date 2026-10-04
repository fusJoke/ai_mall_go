<!-- src\views\supplier\products\index.vue — 供应商盲盒商品列表（任务 6.10；5.3 批次化增强）。
     生命周期 tab 筛选 + 生命周期徽章 + 销售进度条 + 「再来一批」（复制历史批次）+ 上下架。 -->
<template>
    <div class="page">
        <div class="toolbar">
            <h2 class="title">{{ t('supplier.products.title') }}</h2>
            <el-button type="primary" @click="router.push('/supplier/products/edit')">
                {{ t('supplier.products.create') }}
            </el-button>
        </div>

        <el-tabs v-model="activeLifecycle" @tab-change="onTabChange">
            <el-tab-pane :label="t('supplier.products.tabAll')" name="all" />
            <el-tab-pane :label="t('supplier.products.lifecycleSelling')" name="selling" />
            <el-tab-pane :label="t('supplier.products.lifecycleSoldOut')" name="sold_out" />
            <el-tab-pane :label="t('supplier.products.lifecycleSettled')" name="settled" />
        </el-tabs>

        <el-table v-loading="loading" :data="items" stripe>
            <el-table-column prop="id" :label="t('supplier.products.columns.id')" width="64" />
            <el-table-column :label="t('supplier.products.columns.cover')" width="80">
                <template #default="{ row }">
                    <img v-if="row.cover" :src="row.cover" class="cover" />
                    <span v-else class="cover-fallback">🏀</span>
                </template>
            </el-table-column>
            <el-table-column prop="name" :label="t('supplier.products.columns.name')" min-width="160" show-overflow-tooltip />
            <el-table-column :label="t('supplier.products.columns.price')" width="110">
                <template #default="{ row }">¥{{ Number(row.price).toFixed(2) }}</template>
            </el-table-column>
            <el-table-column :label="t('supplier.products.columns.onSale')" width="100">
                <template #default="{ row }">
                    <el-tag :type="row.on_sale ? 'success' : 'info'" size="small">
                        {{ row.on_sale ? t('supplier.products.action.onSale') : t('supplier.products.action.offSale') }}
                    </el-tag>
                </template>
            </el-table-column>
            <el-table-column :label="t('supplier.products.columns.lifecycle')" width="100">
                <template #default="{ row }">
                    <el-tag :type="lifecycleTagType[lcOf(row)]" size="small" effect="plain">
                        {{ t(`supplier.products.lifecycle${LC_LABEL[lcOf(row)]}`) }}
                    </el-tag>
                </template>
            </el-table-column>
            <el-table-column :label="t('supplier.products.columns.progress')" width="150">
                <template #default="{ row }">
                    <template v-if="statOf(row)">
                        <el-progress
                            :percentage="percentOf(row)"
                            :stroke-width="8"
                            :show-text="false"
                            class="progress"
                        />
                        <span class="progress-text">{{ statOf(row)!.sold }} / {{ statOf(row)!.sold + statOf(row)!.remaining }}</span>
                    </template>
                    <span v-else class="progress-text">—</span>
                </template>
            </el-table-column>
            <el-table-column :label="t('user.orders.action')" width="200" fixed="right">
                <template #default="{ row }">
                    <el-button link type="primary" @click="router.push(`/supplier/products/edit/${row.id}`)">
                        {{ t('supplier.products.edit') }}
                    </el-button>
                    <el-button
                        v-if="row.lifecycle === 'selling'"
                        link
                        :type="row.on_sale ? 'warning' : 'success'"
                        @click="toggle(row)"
                    >
                        {{ row.on_sale ? t('supplier.products.action.offSale') : t('supplier.products.action.onSale') }}
                    </el-button>
                    <el-button
                        v-else
                        link
                        type="success"
                        @click="duplicate(row)"
                    >
                        {{ t('supplier.products.action.duplicate') }}
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
    </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import {
    supplierProductDuplicate,
    supplierProductList,
    supplierProductToggleOnSale,
    type SupplierBlindBox,
    type SupplierBoxLifecycle,
    type SupplierPoolStat,
} from '/@/api/supplier/product'
import { useSupplierInfo } from '/@/stores/supplier/supplierInfo'

const { t } = useI18n()
const router = useRouter()
const supplierInfo = useSupplierInfo()

/** lifecycle -> i18n key 后缀（lifecycleSelling / lifecycleSoldOut / lifecycleSettled）。 */
const LC_LABEL: Record<SupplierBoxLifecycle, string> = {
    selling: 'Selling',
    sold_out: 'SoldOut',
    settled: 'Settled',
}

const lifecycleTagType: Record<SupplierBoxLifecycle, 'success' | 'warning' | 'info'> = {
    selling: 'success',
    sold_out: 'warning',
    settled: 'info',
}

const items = ref<SupplierBlindBox[]>([])
const stats = ref<Record<string, SupplierPoolStat>>({})
const total = ref(0)
const page = ref(1)
const pageSize = 10
const loading = ref(false)
const activeLifecycle = ref<'all' | SupplierBoxLifecycle>('all')

function statOf(row: SupplierBlindBox): SupplierPoolStat | undefined {
    return stats.value[String(row.id)]
}

/** el-table 插槽 row 是 any，经此收敛为 lifecycle 类型键。 */
function lcOf(row: SupplierBlindBox): SupplierBoxLifecycle {
    return row.lifecycle
}

function percentOf(row: SupplierBlindBox): number {
    const st = statOf(row)
    const totalNum = st ? st.sold + st.remaining : 0
    return totalNum ? Math.round((st!.sold / totalNum) * 100) : 0
}

function requireLogin(): boolean {
    if (supplierInfo.token) return true
    router.push('/supplier/login')
    return false
}

async function load() {
    if (!requireLogin()) return
    loading.value = true
    try {
        const { data } = await supplierProductList({
            lifecycle: activeLifecycle.value === 'all' ? undefined : activeLifecycle.value,
            page: page.value,
            page_size: pageSize,
        })
        items.value = data.items ?? []
        stats.value = data.stats ?? {}
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

async function toggle(row: SupplierBlindBox) {
    try {
        await supplierProductToggleOnSale(row.id, !row.on_sale)
        ElMessage.success(t('supplier.products.action.onSale'))
        await load()
    } catch (err) {
        handle401(err)
    }
}

/** 再来一批：复制历史批次 → 跳编辑页重填库存（后端 stock=0 + 默认下架）。 */
async function duplicate(row: SupplierBlindBox) {
    try {
        const { data } = await supplierProductDuplicate(row.id)
        ElMessage.success(t('supplier.products.duplicateSuccess'))
        router.push(`/supplier/products/edit/${data.id}`)
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
    margin-bottom: 16px;
}
.title {
    margin: 0;
    font-size: 20px;
    color: #0f172a;
}
.cover {
    width: 44px;
    height: 44px;
    border-radius: 8px;
    object-fit: cover;
}
.cover-fallback {
    display: inline-flex;
    width: 44px;
    height: 44px;
    border-radius: 8px;
    background: #f1f5f9;
    align-items: center;
    justify-content: center;
}
.pager {
    display: flex;
    justify-content: flex-end;
    margin-top: 16px;
}
.progress {
    width: 100%;
}
.progress-text {
    font-size: 12px;
    color: #64748b;
}
</style>
