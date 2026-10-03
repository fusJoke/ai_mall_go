<template>
    <div class="admin-log">
        <!-- 顶部工具栏：搜索 + 批量删除 -->
        <div class="toolbar">
            <el-input
                v-model="searchKeyword"
                :placeholder="$t('log.searchPlaceholder')"
                clearable
                style="width: 280px"
                @keyup.enter="handleSearch"
                @clear="handleSearch"
            />
            <el-button type="primary" @click="handleSearch">{{ $t('actions.search') }}</el-button>
            <el-button type="danger" :disabled="selectedIds.length === 0" @click="handleBatchDelete">
                {{ $t('log.action.batchDelete') }}
            </el-button>
        </div>

        <!-- 日志列表 -->
        <el-table v-loading="loading" :data="items" border stripe @selection-change="onSelectionChange">
            <el-table-column type="selection" width="48" />
            <el-table-column prop="id" :label="$t('log.columns.id')" width="80" sortable />
            <el-table-column prop="username" :label="$t('log.columns.username')" width="120" />
            <el-table-column prop="title" :label="$t('log.columns.title')" min-width="140">
                <template #default="{ row }">
                    <el-tag size="small" :type="tagType(row.title)">{{ row.title }}</el-tag>
                </template>
            </el-table-column>
            <el-table-column prop="url" :label="$t('log.columns.url')" min-width="200" show-overflow-tooltip />
            <el-table-column prop="ip" :label="$t('log.columns.ip')" width="130" />
            <el-table-column prop="createtime" :label="$t('log.columns.createtime')" width="170" sortable />
            <el-table-column :label="$t('actions.operate')" width="160" fixed="right">
                <template #default="{ row }">
                    <el-button link type="primary" @click="openDetailDialog(row)">{{ $t('log.action.detail') }}</el-button>
                    <el-button link type="danger" @click="handleDelete(row)">{{ $t('log.action.delete') }}</el-button>
                </template>
            </el-table-column>
        </el-table>

        <!-- 分页 -->
        <div class="pager">
            <el-pagination
                v-model:current-page="page"
                v-model:page-size="pageSize"
                :total="total"
                :page-sizes="[20, 50, 100, 200]"
                layout="total, sizes, prev, pager, next, jumper"
                @current-change="loadList"
                @size-change="loadList"
            />
        </div>

        <!-- 详情弹窗 -->
        <el-dialog v-model="detailDialogVisible" :title="$t('log.dialog.detail')" width="640px">
            <el-descriptions v-if="detailRow" :column="2" border>
                <el-descriptions-item :label="$t('log.columns.id')">{{ detailRow.id }}</el-descriptions-item>
                <el-descriptions-item :label="$t('log.columns.adminId')">{{ detailRow.admin_id }}</el-descriptions-item>
                <el-descriptions-item :label="$t('log.columns.username')">{{ detailRow.username }}</el-descriptions-item>
                <el-descriptions-item :label="$t('log.columns.ip')">{{ detailRow.ip }}</el-descriptions-item>
                <el-descriptions-item :label="$t('log.columns.title')" :span="2">{{ detailRow.title }}</el-descriptions-item>
                <el-descriptions-item :label="$t('log.columns.url')" :span="2">
                    <el-tag size="small">{{ detailRow.url }}</el-tag>
                </el-descriptions-item>
                <el-descriptions-item :label="$t('log.columns.createtime')" :span="2">{{ detailRow.createtime }}</el-descriptions-item>
                <el-descriptions-item :label="$t('log.columns.useragent')" :span="2">
                    <span class="useragent-text">{{ detailRow.useragent }}</span>
                </el-descriptions-item>
            </el-descriptions>
        </el-dialog>
    </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { adminLogList, adminLogDelete, adminLogBatchDelete, type AdminLog } from '/@/api/adminLog'

defineOptions({ name: 'adminLog' })

const { t } = useI18n()

// --- 列表 state ---

const searchKeyword = ref<string>('')
const page = ref<number>(1)
const pageSize = ref<number>(20)
const total = ref<number>(0)
const items = ref<AdminLog[]>([])
const loading = ref<boolean>(false)
const selectedIds = ref<number[]>([])

async function loadList() {
    loading.value = true
    try {
        const { data } = await adminLogList({
            page: page.value,
            page_size: pageSize.value,
            keyword: searchKeyword.value,
        })
        items.value = data.items ?? []
        total.value = data.total ?? 0
    } finally {
        loading.value = false
    }
}

function handleSearch() {
    page.value = 1
    loadList()
}

function onSelectionChange(rows: AdminLog[]) {
    selectedIds.value = rows.map((r) => r.id)
}

/**
 * 操作标题的语义色（登录/退出 → success；写操作 → warning；读操作 → info）
 */
function tagType(title: string): 'success' | 'warning' | 'info' {
    if (title.includes('登录') || title.includes('退出')) return 'success'
    if (title.includes('新增') || title.includes('编辑') || title.includes('修改') || title.includes('保存') || title.includes('删除'))
        return 'warning'
    return 'info'
}

// --- 详情弹窗 ---

const detailDialogVisible = ref<boolean>(false)
const detailRow = ref<AdminLog | null>(null)

function openDetailDialog(row: AdminLog) {
    detailRow.value = row
    detailDialogVisible.value = true
}

// --- 行内操作 ---

async function handleDelete(row: AdminLog) {
    await ElMessageBox.confirm(t('log.confirm.delete'), '', { type: 'warning' })
    await adminLogDelete(row.id)
    ElMessage.success(t('log.action.delete'))
    loadList()
}

async function handleBatchDelete() {
    if (selectedIds.value.length === 0) return
    await ElMessageBox.confirm(t('log.confirm.batchDelete'), '', { type: 'warning' })
    const { data } = await adminLogBatchDelete(selectedIds.value)
    ElMessage.success(`${t('log.action.batchDelete')}: deleted=${data.deleted}`)
    loadList()
}

onMounted(() => {
    loadList()
})
</script>

<style scoped lang="scss">
.admin-log {
    padding: 16px;
}
.toolbar {
    display: flex;
    gap: 8px;
    margin-bottom: 16px;
    align-items: center;
}
.pager {
    margin-top: 16px;
    display: flex;
    justify-content: flex-end;
}
.useragent-text {
    font-size: 12px;
    word-break: break-all;
}
</style>
