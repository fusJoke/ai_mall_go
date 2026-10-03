<!-- src\views\admin\promotion\index.vue — admin 促销活动只读列表页（任务 6.11）。
     说明：spec 中活动的启停 / 改价 / 删除由供应商端自行管理，admin 端是
     查看 / 联动视图（禁供应商 → C 端列表过滤由 service 层保证），
     数据来自 GET /admin/promotion/list（internal/router/admin/mall.go）。 -->
<template>
    <div class="page">
        <el-table v-loading="loading" :data="items" stripe>
            <el-table-column prop="id" label="ID" width="64" />
            <el-table-column prop="supplier_id" :label="t('admin.mall.promotion.columns.supplierId')" width="110" />
            <el-table-column prop="blind_box_id" :label="t('admin.mall.promotion.columns.blindBoxId')" width="110" />
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
                    <el-tag :type="row.status === 'active' ? 'success' : 'info'" size="small">{{ row.status }}</el-tag>
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
import { mallPromotionList, type MallPromotion } from '/@/api/admin/mall'

const { t } = useI18n()

const items = ref<MallPromotion[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 20
const loading = ref(false)

async function load() {
    loading.value = true
    try {
        const { data } = await mallPromotionList({ page: page.value, page_size: pageSize })
        items.value = data.items ?? []
        total.value = data.total
    } finally {
        loading.value = false
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
.pager {
    display: flex;
    justify-content: flex-end;
    margin-top: 16px;
}
</style>
