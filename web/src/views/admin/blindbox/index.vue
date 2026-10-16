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
    } catch (err) {
        // 加载失败要有反馈（否则请求 reject 变成 unhandled rejection，用户只看到空表），
        // 但**刻意不清空 items/total**：清空会把用户正在看的那一页数据抹掉，
        // 而 page 还停在原页码，分页器会显示成「第 5 页 / 共 0 条」这种更糟的状态，
        // 用户连翻回可用页都做不到。保留上一次的好数据，至少表还能看。
        // 注意 request 拦截器已对 HTTP/业务错误弹过默认文案，这里不重复弹。
        console.error('[admin/blindbox] 列表加载失败，保留上一次的数据', err)
    } finally {
        loading.value = false
    }
}

// 三个开关（启停 / 上架 / 精选）原先都是「裸 await」：请求失败时
// unhandled rejection 直接冒泡，既没有 ElMessage 反馈、也不会重新拉列表，
// 结果是「本地开关已经翻过去了、后端其实没改、界面还不刷新」的假成功。
//
// 收敛到一个 helper 而不是复制三段 try/catch，关键差别在 await load() 的位置：
// 它必须留在 catch **之外**。若放进同一个 try，刷新失败会被 catch 一起捕获，
// 于是「操作其实已经成功、只是列表没刷新出来」被误报成「操作失败」。
async function runToggle(action: () => Promise<unknown>) {
    try {
        await action()
    } catch (err) {
        console.error('[admin/blindbox] 开关操作失败', err)
        ElMessage.error('Error')
        return
    }

    ElMessage.success('OK')
    await load()
}

async function toggleStatus(row: MallBlindBox) {
    await runToggle(() => mallBlindBoxToggleStatus(row.id))
}

async function toggleOnSale(row: MallBlindBox) {
    await runToggle(() => mallBlindBoxToggleOnSale(row.id))
}

async function toggleFeatured(row: MallBlindBox) {
    await runToggle(() => mallBlindBoxToggleFeatured(row.id))
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
