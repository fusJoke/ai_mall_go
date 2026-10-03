<template>
    <div class="admin-group">
        <!-- 顶部工具栏：搜索 + 新增 -->
        <div class="toolbar">
            <el-input v-model="searchKeyword" :placeholder="$t('group.searchPlaceholder')" clearable style="width: 280px" @input="onFilter" />
            <el-button type="primary" @click="openCreateDialog">{{ $t('group.dialog.create') }}</el-button>
        </div>

        <!-- 角色分组列表 -->
        <el-table v-loading="loading" :data="filteredItems" border stripe>
            <el-table-column prop="id" :label="$t('group.columns.id')" width="70" />
            <el-table-column prop="name" :label="$t('group.columns.name')" min-width="140">
                <template #default="{ row }">
                    <el-tag :type="row.id === 1 ? 'danger' : 'primary'" size="small">{{ row.name }}</el-tag>
                </template>
            </el-table-column>
            <el-table-column prop="description" :label="$t('group.columns.description')" min-width="220" show-overflow-tooltip />
            <el-table-column :label="$t('group.columns.rules')" width="100">
                <template #default="{ row }">
                    <el-tag size="small" type="info">{{ row.rules.length }}</el-tag>
                </template>
            </el-table-column>
            <el-table-column :label="$t('group.columns.status')" width="100">
                <template #default="{ row }">
                    <el-tag :type="row.status === 1 ? 'success' : 'info'">
                        {{ row.status === 1 ? $t('group.statusOptions.enabled') : $t('group.statusOptions.disabled') }}
                    </el-tag>
                </template>
            </el-table-column>
            <el-table-column prop="updated_at" :label="$t('group.columns.updatedAt')" min-width="160">
                <template #default="{ row }">
                    <span>{{ row.updated_at || '—' }}</span>
                </template>
            </el-table-column>
            <el-table-column :label="$t('actions.operate')" width="220" fixed="right">
                <template #default="{ row }">
                    <el-button link type="primary" @click="openEditDialog(row)">{{ $t('group.action.edit') }}</el-button>
                    <el-button link :type="row.status === 1 ? 'warning' : 'success'" @click="handleToggleStatus(row)">
                        {{ $t('group.action.toggleStatus') }}
                    </el-button>
                    <el-button link type="danger" :disabled="row.id === 1" @click="handleDelete(row)">
                        {{ $t('group.action.delete') }}
                    </el-button>
                </template>
            </el-table-column>
        </el-table>

        <!-- 新建 / 编辑弹窗（含 RBAC 权限树） -->
        <el-dialog
            v-model="formDialogVisible"
            :title="formDialogMode === 'create' ? $t('group.dialog.create') : $t('group.dialog.edit')"
            width="640px"
            top="6vh"
            @open="loadRuleTree"
        >
            <el-form ref="formRef" :model="form" :rules="formRules" label-width="90px">
                <el-form-item :label="$t('group.columns.name')" prop="name">
                    <el-input v-model="form.name" :disabled="formDialogMode === 'edit' && form.id === 1" />
                </el-form-item>
                <el-form-item :label="$t('group.columns.description')">
                    <el-input v-model="form.description" type="textarea" :rows="2" />
                </el-form-item>
                <el-form-item :label="$t('group.columns.status')">
                    <el-switch
                        v-model="form.status"
                        :active-value="1"
                        :inactive-value="0"
                        inline-prompt
                        :active-text="$t('group.statusOptions.enabled')"
                        :inactive-text="$t('group.statusOptions.disabled')"
                        :disabled="formDialogMode === 'edit' && form.id === 1"
                    />
                </el-form-item>
                <el-form-item :label="$t('group.columns.rules')">
                    <div class="rule-tree-wrap">
                        <el-tree
                            ref="ruleTreeRef"
                            v-loading="ruleTreeLoading"
                            :data="ruleTree"
                            node-key="id"
                            show-checkbox
                            default-expand-all
                            :props="{ label: 'title', children: 'children' }"
                        >
                            <template #default="{ data }">
                                <span class="rule-tree-node">
                                    {{ data.title }}
                                    <el-tag size="small" :type="data.type === 'dir' ? 'warning' : data.type === 'menu' ? 'primary' : 'info'">
                                        {{ typeLabel(data.type) }}
                                    </el-tag>
                                </span>
                            </template>
                        </el-tree>
                    </div>
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
import { computed, nextTick, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox, type FormInstance, type FormRules, type TreeInstance } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { groupList, groupGet, groupCreate, groupEdit, groupToggleStatus, groupDelete, groupRuleTree, type GroupRuleTreeResponse } from '/@/api/group'
import type { AdminGroup } from '/@/mock/adminGroups'

defineOptions({ name: 'adminGroup' })

const { t } = useI18n()

// --- 列表 state ---

const searchKeyword = ref<string>('')
const items = ref<AdminGroup[]>([])
const loading = ref<boolean>(false)

const filteredItems = computed(() => {
    const keyword = searchKeyword.value.trim().toLowerCase()
    if (!keyword) return items.value
    return items.value.filter((group) => group.name.toLowerCase().includes(keyword) || group.description.toLowerCase().includes(keyword))
})

const onFilter = () => {
    // computed 自动联动，无需额外处理
}

async function loadList() {
    loading.value = true
    try {
        const { data } = await groupList()
        items.value = data ?? []
    } finally {
        loading.value = false
    }
}

// typeLabel 把规则类型映射到 i18n 文案（与菜单规则页共用同一套词）
function typeLabel(value: string): string {
    if (value === 'dir') return t('rule.typeOptions.dir')
    if (value === 'menu') return t('rule.typeOptions.menu')
    if (value === 'node') return t('rule.typeOptions.node')
    return value
}

// --- 权限树 ---

const ruleTreeRef = ref<TreeInstance>()
const ruleTree = ref<GroupRuleTreeResponse>([])
const ruleTreeLoading = ref<boolean>(false)

async function loadRuleTree() {
    ruleTreeLoading.value = true
    try {
        const { data } = await groupRuleTree()
        ruleTree.value = data ?? []
        await nextTick()
        applyCheckedKeys()
    } finally {
        ruleTreeLoading.value = false
    }
}

/**
 * 权限树回显：只对叶子节点 setCheckedKeys —— el-tree 级联勾选会自动
 * 推导父节点（全选子节点→勾选父节点，部分勾选→半选），直接对含子节点的
 * 目录 id setChecked 会误勾其全部子孙。
 */
function applyCheckedKeys() {
    if (!ruleTreeRef.value) return
    const parentIds = new Set<number>()
    const walk = (nodes: typeof ruleTree.value) => {
        for (const node of nodes) {
            if (node.children?.length) {
                parentIds.add(node.id)
                walk(node.children)
            }
        }
    }
    walk(ruleTree.value)
    const leafIds = form.rules.filter((id) => !parentIds.has(id))
    ruleTreeRef.value.setCheckedKeys(leafIds, false)
}

/**
 * 收集勾选结果：checked + halfChecked（半选的父目录 / 菜单也要存，否则回显时树不完整）
 */
function collectCheckedRules(): number[] {
    if (!ruleTreeRef.value) return []
    const checked = (ruleTreeRef.value.getCheckedKeys() as number[]).map(Number)
    const halfChecked = (ruleTreeRef.value.getHalfCheckedKeys() as number[]).map(Number)
    return [...checked, ...halfChecked]
}

// --- 创建 / 编辑 弹窗 ---

const formDialogVisible = ref<boolean>(false)
const formDialogMode = ref<'create' | 'edit'>('create')
const submitting = ref<boolean>(false)
const formRef = ref<FormInstance>()

interface GroupForm {
    id?: number
    name: string
    description: string
    status: 0 | 1
    rules: number[]
}

const form = reactive<GroupForm>({
    name: '',
    description: '',
    status: 1,
    rules: [],
})

const formRules = reactive<FormRules<GroupForm>>({
    name: [{ required: true, message: () => t('group.columns.name'), trigger: 'blur' }],
})

function resetForm() {
    form.id = undefined
    form.name = ''
    form.description = ''
    form.status = 1
    form.rules = []
}

function openCreateDialog() {
    resetForm()
    formDialogMode.value = 'create'
    formDialogVisible.value = true
}

async function openEditDialog(row: AdminGroup) {
    resetForm()
    formDialogMode.value = 'edit'
    try {
        const { data } = await groupGet(row.id)
        form.id = data.id
        form.name = data.name
        form.description = data.description
        form.status = data.status
        form.rules = data.rules ?? []
    } catch {
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
            await groupCreate({
                name: form.name,
                description: form.description,
                status: form.status,
                rules: collectCheckedRules(),
            })
            ElMessage.success(t('group.dialog.create'))
        } else {
            await groupEdit({
                id: form.id,
                name: form.name,
                description: form.description,
                status: form.status,
                rules: collectCheckedRules(),
            })
            ElMessage.success(t('group.dialog.edit'))
        }
        formDialogVisible.value = false
        loadList()
    } finally {
        submitting.value = false
    }
}

// --- 行内操作 ---

async function handleToggleStatus(row: AdminGroup) {
    try {
        await groupToggleStatus(row.id)
    } catch {
        return
    }
    ElMessage.success(t('group.action.toggleStatus'))
    loadList()
}

async function handleDelete(row: AdminGroup) {
    await ElMessageBox.confirm(t('group.confirm.delete'), '', { type: 'warning' })
    await groupDelete(row.id)
    ElMessage.success(t('group.action.delete'))
    loadList()
}

onMounted(() => {
    loadList()
})
</script>

<style scoped lang="scss">
.admin-group {
    padding: 16px;
}
.toolbar {
    display: flex;
    gap: 8px;
    margin-bottom: 16px;
    align-items: center;
}
.rule-tree-wrap {
    width: 100%;
    max-height: 340px;
    overflow: auto;
    border: 1px solid var(--el-border-color-lighter);
    border-radius: var(--el-border-radius-base);
    padding: 8px;
}
.rule-tree-node {
    display: flex;
    align-items: center;
    gap: 6px;
}
</style>
