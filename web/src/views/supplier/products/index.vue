<!-- src\views\supplier\products\index.vue — 供应商盲盒商品列表（任务 6.10）。
     表格 + 新建（跳编辑页）+ 上下架 + 编辑入口。 -->
<template>
    <div class="page">
        <div class="toolbar">
            <h2 class="title">{{ t('supplier.products.title') }}</h2>
            <el-button type="primary" @click="router.push('/supplier/products/edit')">
                {{ t('supplier.products.create') }}
            </el-button>
        </div>

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
            <el-table-column :label="t('supplier.products.columns.onSale')" width="110">
                <template #default="{ row }">
                    <el-tag :type="row.on_sale ? 'success' : 'info'" size="small">
                        {{ row.on_sale ? t('supplier.products.action.onSale') : t('supplier.products.action.offSale') }}
                    </el-tag>
                </template>
            </el-table-column>
            <el-table-column :label="t('supplier.products.columns.status')" width="100">
                <template #default="{ row }">
                    <el-tag :type="row.status === 'active' ? 'success' : 'danger'" size="small" effect="plain">
                        {{ row.status === 'active' ? t('supplier.products.statusActive') : t('supplier.products.statusDisabled') }}
                    </el-tag>
                </template>
            </el-table-column>
            <el-table-column :label="t('user.orders.action')" width="170" fixed="right">
                <template #default="{ row }">
                    <el-button link type="primary" @click="router.push(`/supplier/products/edit/${row.id}`)">
                        {{ t('supplier.products.edit') }}
                    </el-button>
                    <el-button link :type="row.on_sale ? 'warning' : 'success'" @click="toggle(row)">
                        {{ row.on_sale ? t('supplier.products.action.offSale') : t('supplier.products.action.onSale') }}
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
import { supplierProductList, supplierProductToggleOnSale, type SupplierBlindBox } from '/@/api/supplier/product'
import { useSupplierInfo } from '/@/stores/supplier/supplierInfo'

const { t } = useI18n()
const router = useRouter()
const supplierInfo = useSupplierInfo()

const items = ref<SupplierBlindBox[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 10
const loading = ref(false)

function requireLogin(): boolean {
    if (supplierInfo.token) return true
    router.push('/supplier/login')
    return false
}

async function load() {
    if (!requireLogin()) return
    loading.value = true
    try {
        const { data } = await supplierProductList({ page: page.value, page_size: pageSize })
        items.value = data.items ?? []
        total.value = data.total
    } catch (err) {
        handle401(err)
    } finally {
        loading.value = false
    }
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
</style>
