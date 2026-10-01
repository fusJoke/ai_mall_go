<template>
    <div class="admin-rule">
        <!-- 顶部工具栏：搜索 + 新增 + 批量删除 -->
        <div class="toolbar">
            <el-input
                v-model="searchKeyword"
                :placeholder="$t('rule.searchPlaceholder')"
                clearable
                style="width: 280px"
                @keyup.enter="handleSearch"
            />
            <el-button type="primary" @click="handleSearch">{{ $t('actions.search') }}</el-button>
            <el-button type="primary" @click="openCreateDialog">{{ $t('rule.dialog.create') }}</el-button>
            <el-button type="danger" :disabled="selectedIds.length === 0" @click="handleBatchDelete">
                {{ $t('rule.action.batchDelete') }}
            </el-button>
        </div>

        <!-- 规则列表 -->
        <el-table v-loading="loading" :data="items" border stripe row-key="id" @selection-change="onSelectionChange">
            <el-table-column type="selection" width="48" />
            <el-table-column prop="id" :label="$t('rule.columns.id')" width="80" />
            <el-table-column prop="pid" :label="$t('rule.columns.pid')" width="80" />
            <el-table-column :label="$t('rule.columns.type')" width="100">
                <template #default="{ row }">
                    <el-tag :type="row.type === 'menu' ? 'primary' : row.type === 'dir' ? 'warning' : 'info'">
                        {{ typeLabel(row.type) }}
                    </el-tag>
                </template>
            </el-table-column>
            <el-table-column prop="title" :label="$t('rule.columns.title')" min-width="160" />
            <el-table-column prop="name" :label="$t('rule.columns.name')" min-width="160" />
            <el-table-column prop="path" :label="$t('rule.columns.path')" min-width="160" />
            <el-table-column prop="icon" :label="$t('rule.columns.icon')" min-width="120" />
            <el-table-column prop="weigh" :label="$t('rule.columns.weigh')" width="80" />
            <el-table-column :label="$t('rule.columns.status')" width="100">
                <template #default="{ row }">
                    <el-tag :type="row.status === 1 ? 'success' : 'info'">
                        {{ row.status === 1 ? $t('rule.statusOptions.enabled') : $t('rule.statusOptions.disabled') }}
                    </el-tag>
                </template>
            </el-table-column>
            <el-table-column :label="$t('rule.columns.updatedAt')" min-width="160">
                <template #default="{ row }">
                    <span>{{ row.updated_at || '—' }}</span>
                </template>
            </el-table-column>
            <el-table-column :label="$t('actions.operate')" width="240" fixed="right">
                <template #default="{ row }">
                    <el-button link type="primary" @click="openEditDialog(row)">{{ $t('rule.action.edit') }}</el-button>
                    <el-button link :type="row.status === 1 ? 'warning' : 'success'" @click="handleToggleStatus(row)">
                        {{ $t('rule.action.toggleStatus') }}
                    </el-button>
                    <el-button link type="danger" @click="handleDelete(row)">
                        {{ $t('rule.action.delete') }}
                    </el-button>
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

        <!-- 新建 / 编辑弹窗 -->
        <el-dialog
            v-model="formDialogVisible"
            :title="formDialogMode === 'create' ? $t('rule.dialog.create') : $t('rule.dialog.edit')"
            width="640px"
            @closed="resetForm"
            @open="loadAllRules"
        >
            <el-form ref="formRef" :model="form" :rules="formRules" label-width="120px">
                <el-form-item :label="$t('rule.columns.title')" prop="title">
                    <el-input v-model="form.title" />
                </el-form-item>
                <el-form-item :label="$t('rule.columns.name')" prop="name">
                    <el-input v-model="form.name" />
                </el-form-item>
                <el-form-item :label="$t('rule.columns.pid')" prop="pid">
                    <el-select v-model="form.pid" :placeholder="$t('rule.columns.pid')" style="width: 100%">
                        <el-option :label="`顶级 (pid=0)`" :value="0" />
                        <el-option
                            v-for="opt in parentOptions"
                            :key="opt.id"
                            :label="`${opt.id} · ${opt.title}`"
                            :value="opt.id"
                            :disabled="formDialogMode === 'edit' && opt.id === form.id"
                        />
                    </el-select>
                </el-form-item>
                <el-form-item :label="$t('rule.columns.type')" prop="type">
                    <el-select v-model="form.type" style="width: 100%">
                        <el-option :label="$t('rule.typeOptions.dir')" value="dir" />
                        <el-option :label="$t('rule.typeOptions.menu')" value="menu" />
                        <el-option :label="$t('rule.typeOptions.node')" value="node" />
                    </el-select>
                </el-form-item>
                <el-form-item :label="$t('rule.columns.path')">
                    <el-input v-model="form.path" />
                </el-form-item>
                <el-form-item :label="$t('rule.columns.icon')">
                    <el-input v-model="form.icon" />
                </el-form-item>
                <el-form-item :label="$t('rule.columns.weigh')">
                    <el-input-number v-model="form.weigh" :min="0" :max="9999" />
                </el-form-item>
                <el-form-item :label="$t('rule.columns.status')">
                    <el-switch
                        v-model="form.status"
                        :active-value="1"
                        :inactive-value="0"
                        inline-prompt
                        :active-text="$t('rule.statusOptions.enabled')"
                        :inactive-text="$t('rule.statusOptions.disabled')"
                    />
                </el-form-item>
            </el-form>
            <template #footer>
                <el-button @click="formDialogVisible = false">{{ $t('actions.cancel') }}</el-button>
                <el-button type="primary" :loading="submitting" @click="submitForm">
                    {{ $t('actions.submit') }}
                </el-button>
            </template>
        </el-dialog>
    </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox, type FormInstance, type FormRules } from 'element-plus'
import { useI18n } from 'vue-i18n'

import {
    ruleList,
    ruleGet,
    ruleCreate,
    ruleEdit,
    ruleDelete,
    ruleToggleStatus,
    ruleBatchDelete,
    ruleListAll,
    type AdminRule,
    type AdminRuleType,
} from '/@/api/rule'

defineOptions({ name: 'adminRule' })

const { t } = useI18n()

// --- 列表 state ---

const searchKeyword = ref<string>('')
const page = ref<number>(1)
const pageSize = ref<number>(20)
const total = ref<number>(0)
const items = ref<AdminRule[]>([])
const loading = ref<boolean>(false)
const selectedIds = ref<number[]>([])

async function loadList() {
    loading.value = true
    try {
        const { data } = await ruleList({
            page: page.value,
            page_size: pageSize.value,
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

function onSelectionChange(rows: AdminRule[]) {
    selectedIds.value = rows.map((r) => r.id)
}

// typeLabel 把枚举映射到 i18n 文案；空 / 未知值兜底回 type 字面。
function typeLabel(value: AdminRuleType): string {
    if (value === 'dir') return t('rule.typeOptions.dir')
    if (value === 'menu') return t('rule.typeOptions.menu')
    if (value === 'node') return t('rule.typeOptions.node')
    return value
}

// --- 父级规则下拉框选项（来自 /admin/rule/all） ---

const parentOptions = ref<AdminRule[]>([])

async function loadAllRules() {
    try {
        const { data } = await ruleListAll()
        parentOptions.value = data ?? []
    } catch {
        parentOptions.value = []
    }
}

// --- 创建 / 编辑 弹窗 ---

const formDialogVisible = ref<boolean>(false)
const formDialogMode = ref<'create' | 'edit'>('create')
const submitting = ref<boolean>(false)
const formRef = ref<FormInstance>()

interface RuleForm {
    id?: number
    pid: number
    type: AdminRuleType
    title: string
    name: string
    path: string
    icon: string
    weigh: number
    status: 0 | 1
}

const form = reactive<RuleForm>({
    pid: 0,
    type: 'menu',
    title: '',
    name: '',
    path: '',
    icon: '',
    weigh: 0,
    status: 1,
})

const formRules = reactive<FormRules<RuleForm>>({
    title: [{ required: true, message: () => t('rule.columns.title'), trigger: 'blur' }],
    name: [{ required: true, message: () => t('rule.columns.name'), trigger: 'blur' }],
})

function resetForm() {
    form.id = undefined
    form.pid = 0
    form.type = 'menu'
    form.title = ''
    form.name = ''
    form.path = ''
    form.icon = ''
    form.weigh = 0
    form.status = 1
}

function openCreateDialog() {
    resetForm()
    formDialogMode.value = 'create'
    formDialogVisible.value = true
}

async function openEditDialog(row: AdminRule) {
    resetForm()
    formDialogMode.value = 'edit'
    try {
        const { data } = await ruleGet(row.id)
        form.id = data.id
        form.pid = data.pid
        form.type = data.type
        form.title = data.title
        form.name = data.name
        form.path = data.path
        form.icon = data.icon
        form.weigh = data.weigh
        form.status = data.status
    } catch {
        // ruleGet 失败时，request 拦截器已经 toast；这里静默吞掉即可。
        return
    }
    formDialogVisible.value = true
}

async function submitForm() {
    if (!formRef.value) return
    try {
        await formRef.value.validate()
    } catch {
        return
    }
    submitting.value = true
    try {
        if (formDialogMode.value === 'create') {
            await ruleCreate({
                pid: form.pid,
                type: form.type,
                title: form.title,
                name: form.name,
                path: form.path,
                icon: form.icon,
                weigh: form.weigh,
                status: form.status,
            })
            ElMessage.success(t('rule.dialog.create'))
        } else {
            await ruleEdit({
                id: form.id,
                pid: form.pid,
                type: form.type,
                title: form.title,
                name: form.name,
                path: form.path,
                icon: form.icon,
                weigh: form.weigh,
                status: form.status,
            })
            ElMessage.success(t('rule.dialog.edit'))
        }
        formDialogVisible.value = false
        loadList()
    } finally {
        submitting.value = false
    }
}

// --- 行内操作 ---

async function handleToggleStatus(row: AdminRule) {
    try {
        await ruleToggleStatus({ id: row.id })
    } catch {
        // 业务错误（如 404 rule.toggle_status.not_found）由 request 拦截器统一 toast。
        return
    }
    ElMessage.success(t('rule.action.toggleStatus'))
    loadList()
}

async function handleDelete(row: AdminRule) {
    await ElMessageBox.confirm(t('rule.confirm.delete'), '', { type: 'warning' })
    try {
        await ruleDelete(row.id)
    } catch {
        // has_children 等业务错误由 request 拦截器 toast。
        return
    }
    ElMessage.success(t('rule.action.delete'))
    loadList()
}

async function handleBatchDelete() {
    if (selectedIds.value.length === 0) return
    await ElMessageBox.confirm(t('rule.confirm.batchDelete'), '', { type: 'warning' })
    const { data } = await ruleBatchDelete({ ids: selectedIds.value })
    ElMessage.success(`${t('rule.action.batchDelete')}: deleted=${data.deleted}, skipped=${data.skipped}`)
    loadList()
}

onMounted(() => {
    loadList()
})
</script>

<style scoped lang="scss">
.admin-rule {
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
</style>
