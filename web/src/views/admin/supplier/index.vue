<!-- src\views\admin\supplier\index.vue — admin 供应商管理页（任务 6.11）。
     列表筛选（status / is_featured / 名称关键字）+ 启停 + 推荐位切换。 -->
<template>
    <div class="page">
        <div class="toolbar">
            <div class="filters">
                <el-select v-model="filters.status" clearable :placeholder="t('admin.mall.supplier.statusFilter')" style="width: 140px">
                    <el-option :label="t('admin.mall.supplier.statusActive')" value="active" />
                    <el-option :label="t('admin.mall.supplier.statusDisabled')" value="disabled" />
                </el-select>
                <el-select v-model="filters.featured" clearable :placeholder="t('admin.mall.supplier.featuredFilter')" style="width: 140px">
                    <el-option :label="t('admin.mall.supplier.featuredYes')" value="true" />
                    <el-option :label="t('admin.mall.supplier.featuredNo')" value="false" />
                </el-select>
                <el-input v-model="filters.name" :placeholder="t('admin.mall.supplier.nameFilter')" clearable style="width: 200px" @keyup.enter="reload" />
                <el-button type="primary" plain @click="reload">{{ t('admin.mall.common.search') }}</el-button>
            </div>
        </div>

        <el-table v-loading="loading" :data="items" stripe>
            <el-table-column prop="id" label="ID" width="64" />
            <el-table-column prop="name" :label="t('admin.mall.supplier.columns.name')" min-width="150" show-overflow-tooltip />
            <el-table-column prop="contact_phone" :label="t('admin.mall.supplier.columns.contactPhone')" width="130" />
            <el-table-column :label="t('admin.mall.supplier.columns.balance')" width="120">
                <template #default="{ row }">¥{{ Number(row.balance).toFixed(2) }}</template>
            </el-table-column>
            <el-table-column :label="t('admin.mall.supplier.columns.totalSales')" width="130">
                <template #default="{ row }">¥{{ Number(row.total_sales).toFixed(2) }}</template>
            </el-table-column>
            <el-table-column :label="t('admin.mall.supplier.columns.status')" width="100">
                <template #default="{ row }">
                    <el-tag :type="row.status === 'active' ? 'success' : 'danger'" size="small">
                        {{ row.status === 'active' ? t('admin.mall.supplier.statusActive') : t('admin.mall.supplier.statusDisabled') }}
                    </el-tag>
                </template>
            </el-table-column>
            <el-table-column :label="t('admin.mall.supplier.columns.isFeatured')" width="110">
                <template #default="{ row }">
                    <el-tag :type="row.is_featured ? 'warning' : 'info'" size="small" effect="plain">
                        {{ row.is_featured ? '★' : '—' }}
                    </el-tag>
                </template>
            </el-table-column>
            <el-table-column :label="t('admin.mall.common.actions')" width="180" fixed="right">
                <template #default="{ row }">
                    <el-button link :type="row.status === 'active' ? 'danger' : 'success'" @click="toggleStatus(row)">
                        {{ row.status === 'active' ? t('admin.mall.supplier.disable') : t('admin.mall.supplier.enable') }}
                    </el-button>
                    <el-button link type="warning" @click="toggleFeatured(row)">{{ t('admin.mall.supplier.toggleFeatured') }}</el-button>
                </template>
            </el-table-column>
        </el-table>

        <div class="pager">
            <el-pagination v-model:current-page="page" :page-size="pageSize" :total="total" layout="prev, pager, next" @current-change="load" />
        </div>
    </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import {
    mallSupplierList,
    mallSupplierToggleFeatured,
    mallSupplierToggleStatus,
    type MallSupplier,
} from '/@/api/admin/mall'

const { t } = useI18n()

const items = ref<MallSupplier[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 20
const loading = ref(false)

const filters = reactive<{ status: string; featured: string; name: string }>({
    status: '',
    featured: '',
    name: '',
})

async function load() {
    loading.value = true
    try {
        const { data } = await mallSupplierList({
            page: page.value,
            page_size: pageSize,
            status: filters.status || undefined,
            is_featured: filters.featured || undefined,
            name: filters.name || undefined,
        })
        items.value = data.items ?? []
        total.value = data.total
    } finally {
        loading.value = false
    }
}

function reload() {
    page.value = 1
    load()
}

async function toggleStatus(row: MallSupplier) {
    try {
        await mallSupplierToggleStatus(row.id)
        ElMessage.success('OK')
        await load()
    } catch {
        ElMessage.error('Error')
    }
}

async function toggleFeatured(row: MallSupplier) {
    try {
        await mallSupplierToggleFeatured(row.id)
        ElMessage.success('OK')
        await load()
    } catch {
        ElMessage.error('Error')
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
.filters {
    display: flex;
    gap: 10px;
    flex-wrap: wrap;
}
.pager {
    display: flex;
    justify-content: flex-end;
    margin-top: 16px;
}
</style>
