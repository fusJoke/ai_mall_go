<!-- src\views\admin\blindbox\index.vue — admin 盲盒管理页（任务 6.11）。
     全量列表 + 三类开关（status / on_sale / is_featured）。 -->
<template>
    <div class="page">
        <el-table v-loading="loading" :data="items" stripe>
            <el-table-column prop="id" label="ID" width="64" />
            <el-table-column :label="t('admin.mall.blindbox.columns.cover')" width="80">
                <template #default="{ row }">
                    <img v-if="row.cover" :src="row.cover" class="cover" />
                    <span v-else class="cover-fallback">🏀</span>
                </template>
            </el-table-column>
            <el-table-column prop="name" :label="t('admin.mall.blindbox.columns.name')" min-width="160" show-overflow-tooltip />
            <el-table-column prop="supplier_id" :label="t('admin.mall.blindbox.columns.supplierId')" width="100" />
            <el-table-column :label="t('admin.mall.blindbox.columns.price')" width="110">
                <template #default="{ row }">¥{{ Number(row.price).toFixed(2) }}</template>
            </el-table-column>
            <el-table-column :label="t('admin.mall.blindbox.columns.status')" width="100">
                <template #default="{ row }">
                    <el-tag :type="row.status === 'active' ? 'success' : 'danger'" size="small">
                        {{ row.status === 'active' ? t('admin.mall.supplier.statusActive') : t('admin.mall.supplier.statusDisabled') }}
                    </el-tag>
                </template>
            </el-table-column>
            <el-table-column :label="t('admin.mall.blindbox.columns.onSale')" width="100">
                <template #default="{ row }">
                    <el-tag :type="row.on_sale ? 'success' : 'info'" size="small">
                        {{ row.on_sale ? '✓' : '✗' }}
                    </el-tag>
                </template>
            </el-table-column>
            <el-table-column :label="t('admin.mall.blindbox.columns.isFeatured')" width="110">
                <template #default="{ row }">
                    <el-tag :type="row.is_featured ? 'warning' : 'info'" size="small" effect="plain">
                        {{ row.is_featured ? '★' : '—' }}
                    </el-tag>
                </template>
            </el-table-column>
            <el-table-column :label="t('admin.mall.common.actions')" width="230" fixed="right">
                <template #default="{ row }">
                    <el-button link type="danger" @click="toggleStatus(row)">{{ t('admin.mall.blindbox.toggleStatus') }}</el-button>
                    <el-button link type="warning" @click="toggleOnSale(row)">{{ t('admin.mall.blindbox.toggleOnSale') }}</el-button>
                    <el-button link type="primary" @click="toggleFeatured(row)">{{ t('admin.mall.blindbox.toggleFeatured') }}</el-button>
                </template>
            </el-table-column>
        </el-table>

        <div class="pager">
            <el-pagination v-model:current-page="page" :page-size="pageSize" :total="total" layout="prev, pager, next" @current-change="load" />
        </div>
    </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import {
    mallBlindBoxList,
    mallBlindBoxToggleFeatured,
    mallBlindBoxToggleOnSale,
    mallBlindBoxToggleStatus,
    type MallBlindBox,
} from '/@/api/admin/mall'

const { t } = useI18n()

const items = ref<MallBlindBox[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 20
const loading = ref(false)

async function load() {
    loading.value = true
    try {
        const { data } = await mallBlindBoxList({ page: page.value, page_size: pageSize })
        items.value = data.items ?? []
        total.value = data.total
    } finally {
        loading.value = false
    }
}

async function toggleStatus(row: MallBlindBox) {
    await mallBlindBoxToggleStatus(row.id)
    ElMessage.success('OK')
    await load()
}

async function toggleOnSale(row: MallBlindBox) {
    await mallBlindBoxToggleOnSale(row.id)
    ElMessage.success('OK')
    await load()
}

async function toggleFeatured(row: MallBlindBox) {
    await mallBlindBoxToggleFeatured(row.id)
    ElMessage.success('OK')
    await load()
}

onMounted(load)
</script>

<style scoped lang="scss">
.page {
    background: #fff;
    border-radius: 8px;
    padding: 20px;
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
