<!-- src\views\supplier\seckill\index.vue — 供应商秒杀活动管理（任务 12.5）。
     表格 + 新建 / 编辑跳转 + 启停；卡池 / 时间窗 / 价格校验由后端 422 返回，
     编辑页承接。 -->
<template>
    <div class="page">
        <div class="toolbar">
            <h2 class="title">{{ t('supplier.seckill.title') }}</h2>
            <el-button type="primary" @click="goCreate">{{ t('supplier.seckill.create') }}</el-button>
        </div>

        <el-table v-loading="loading" :data="items" stripe>
            <el-table-column prop="id" :label="t('supplier.seckill.columns.id')" width="64" />
            <el-table-column prop="blind_box_id" :label="t('supplier.seckill.columns.blindBox')" width="100" />
            <el-table-column :label="t('supplier.seckill.columns.seckillPrice')" width="110">
                <template #default="{ row }">¥{{ Number(row.seckill_price).toFixed(2) }}</template>
            </el-table-column>
            <el-table-column prop="total_stock" :label="t('supplier.seckill.columns.totalStock')" width="100" />
            <el-table-column prop="per_user_limit" :label="t('supplier.seckill.columns.perUserLimit')" width="100" />
            <el-table-column :label="t('supplier.seckill.columns.startAt')" min-width="150">
                <template #default="{ row }">{{ new Date(row.start_at).toLocaleString() }}</template>
            </el-table-column>
            <el-table-column :label="t('supplier.seckill.columns.endAt')" min-width="150">
                <template #default="{ row }">{{ new Date(row.end_at).toLocaleString() }}</template>
            </el-table-column>
            <el-table-column :label="t('supplier.seckill.columns.redis')" width="110">
                <template #default="{ row }">
                    <el-tag :type="row.redis_initialized ? 'success' : 'danger'" size="small">
                        {{ row.redis_initialized ? t('supplier.seckill.redisReady') : t('supplier.seckill.redisMissing') }}
                    </el-tag>
                </template>
            </el-table-column>
            <el-table-column :label="t('supplier.seckill.columns.status')" width="100">
                <template #default="{ row }">
                    <el-tag :type="row.status === 'active' ? 'success' : 'info'" size="small">
                        {{ row.status === 'active' ? t('supplier.seckill.statusActive') : t('supplier.seckill.statusDisabled') }}
                    </el-tag>
                </template>
            </el-table-column>
            <el-table-column :label="t('supplier.seckill.action.label')" width="140" fixed="right">
                <template #default="{ row }">
                    <el-button link type="primary" @click="goEdit(row)">{{ t('supplier.seckill.action.edit') }}</el-button>
                    <el-button link type="warning" @click="toggle(row)">{{ t('supplier.seckill.action.toggle') }}</el-button>
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
import { useRouter } from 'vue-router'
import { supplierSeckillList, supplierSeckillToggle, type SupplierSeckill } from '/@/api/supplier/seckill'
import { useSupplierInfo } from '/@/stores/supplier/supplierInfo'

const { t } = useI18n()
const router = useRouter()
const supplierInfo = useSupplierInfo()

const items = ref<SupplierSeckill[]>([])
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
        const { data } = await supplierSeckillList({ page: page.value, page_size: pageSize })
        items.value = data.items ?? []
        total.value = data.total
    } catch (err) {
        handle401(err)
    } finally {
        loading.value = false
    }
}

function goCreate() {
    router.push('/supplier/seckill/edit')
}

function goEdit(row: SupplierSeckill) {
    router.push(`/supplier/seckill/edit/${row.id}`)
}

async function toggle(row: SupplierSeckill) {
    try {
        await supplierSeckillToggle(row.id)
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
.pager {
    display: flex;
    justify-content: flex-end;
    margin-top: 16px;
}
</style>
