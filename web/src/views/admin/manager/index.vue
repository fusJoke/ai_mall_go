<template>
    <div class="admin-manager">
        <!-- 顶部工具栏：搜索 + 新增 -->
        <div class="toolbar">
            <el-input
                v-model="searchKeyword"
                :placeholder="$t('manager.searchPlaceholder')"
                clearable
                style="width: 280px"
                @keyup.enter="handleSearch"
            />
            <el-button type="primary" @click="handleSearch">{{ $t('actions.search') }}</el-button>
            <el-button type="primary" @click="openCreateDialog">{{ $t('manager.dialog.create') }}</el-button>
            <el-button type="danger" :disabled="selectedIds.length === 0" @click="handleBatchDelete">
                {{ $t('manager.action.batchDelete') }}
            </el-button>
        </div>

        <!-- 账号列表 -->
        <el-table v-loading="loading" :data="items" border stripe row-key="id" @selection-change="onSelectionChange">
            <el-table-column type="selection" width="48" />
            <el-table-column prop="username" :label="$t('manager.columns.username')" min-width="120" />
            <el-table-column prop="nickname" :label="$t('manager.columns.nickname')" min-width="120" />
            <el-table-column prop="email" :label="$t('manager.columns.email')" min-width="160" />
            <el-table-column prop="mobile" :label="$t('manager.columns.mobile')" min-width="120" />
            <el-table-column :label="$t('manager.columns.status')" width="100">
                <template #default="{ row }">
                    <el-tag :type="row.status === 1 ? 'success' : 'info'">
                        {{ row.status === 1 ? $t('status.enabled') : $t('status.disabled') }}
                    </el-tag>
                </template>
            </el-table-column>
            <el-table-column :label="$t('manager.columns.lastLoginAt')" min-width="160">
                <template #default="{ row }">
                    <span>{{ row.last_login_at || '—' }}</span>
                </template>
            </el-table-column>
            <el-table-column :label="$t('manager.columns.createdAt')" min-width="160">
                <template #default="{ row }">
                    <span>{{ formatCreatedAt(row.id) }}</span>
                </template>
            </el-table-column>
            <el-table-column :label="$t('actions.operate')" width="320" fixed="right">
                <template #default="{ row }">
                    <el-button link type="primary" @click="openEditDialog(row)">{{ $t('actions.edit') }}</el-button>
                    <el-button link type="primary" @click="openChangePasswordDialog(row)">{{ $t('manager.action.changePassword') }}</el-button>
                    <el-button link :type="row.status === 1 ? 'warning' : 'success'" :disabled="isSelf(row)" @click="handleToggleStatus(row)">
                        {{ $t('manager.action.toggleStatus') }}
                    </el-button>
                    <el-button link type="primary" @click="handleUnlock(row)">{{ $t('manager.action.unlock') }}</el-button>
                    <el-button link type="danger" :disabled="isSelf(row)" @click="handleDelete(row)">
                        {{ $t('manager.action.delete') }}
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
            :title="formDialogMode === 'create' ? $t('manager.dialog.create') : $t('manager.dialog.edit')"
            width="540px"
            @closed="resetForm"
        >
            <el-form ref="formRef" :model="form" :rules="formRules" label-width="100px">
                <el-form-item :label="$t('manager.columns.username')" prop="username">
                    <el-input v-model="form.username" :disabled="formDialogMode === 'edit'" />
                </el-form-item>
                <el-form-item :label="$t('manager.columns.nickname')" prop="nickname">
                    <el-input v-model="form.nickname" />
                </el-form-item>
                <el-form-item :label="$t('manager.columns.email')" prop="email">
                    <el-input v-model="form.email" />
                </el-form-item>
                <el-form-item :label="$t('manager.columns.mobile')" prop="mobile">
                    <el-input v-model="form.mobile" />
                </el-form-item>
                <el-form-item :label="$t('manager.columns.avatar')">
                    <ag-upload type="image" v-model="form.avatar" />
                </el-form-item>
                <el-form-item v-if="formDialogMode === 'create'" :label="$t('manager.columns.password')" prop="password">
                    <el-input v-model="form.password" type="password" show-password />
                </el-form-item>
                <el-form-item v-else :label="$t('manager.columns.password')">
                    <el-input :model-value="''" type="password" show-password disabled :placeholder="$t('manager.dialog.changePassword')" />
                </el-form-item>
                <el-form-item :label="$t('manager.columns.status')">
                    <el-switch
                        v-model="form.status"
                        :active-value="1"
                        :inactive-value="0"
                        inline-prompt
                        :active-text="$t('status.enabled')"
                        :inactive-text="$t('status.disabled')"
                    />
                </el-form-item>
                <el-form-item :label="$t('manager.columns.bio')">
                    <el-input v-model="form.bio" type="textarea" :rows="3" />
                </el-form-item>
            </el-form>
            <template #footer>
                <el-button @click="formDialogVisible = false">{{ $t('actions.cancel') }}</el-button>
                <el-button type="primary" :loading="submitting" @click="submitForm">
                    {{ $t('actions.submit') }}
                </el-button>
            </template>
        </el-dialog>

        <!-- 改密弹窗 -->
        <el-dialog v-model="passwordDialogVisible" :title="$t('manager.dialog.changePassword')" width="420px" @closed="resetPasswordForm">
            <el-form ref="passwordFormRef" :model="passwordForm" :rules="passwordRules" label-width="100px">
                <el-form-item :label="$t('manager.columns.username')">
                    <span>{{ passwordTarget?.username || '' }}</span>
                </el-form-item>
                <el-form-item :label="$t('manager.columns.password')" prop="new_password">
                    <el-input v-model="passwordForm.new_password" type="password" show-password />
                </el-form-item>
            </el-form>
            <template #footer>
                <el-button @click="passwordDialogVisible = false">{{ $t('actions.cancel') }}</el-button>
                <el-button type="primary" :loading="submitting" @click="submitPassword">
                    {{ $t('actions.submit') }}
                </el-button>
            </template>
        </el-dialog>
    </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref, computed } from 'vue'
import { ElMessage, ElMessageBox, type FormInstance, type FormRules } from 'element-plus'
import { useI18n } from 'vue-i18n'

import agUpload from '/@/components/agInput/components/agUpload.vue'
import {
    adminList,
    adminCreate,
    adminEdit,
    adminDelete,
    adminChangePassword,
    adminToggleStatus,
    adminUnlock,
    adminBatchDelete,
    type AdminInfo,
} from '/@/api/admin'
import { useAdminInfo } from '/@/stores/adminInfo'

defineOptions({ name: 'adminManager' })

const { t } = useI18n()
const adminInfo = useAdminInfo()

// 当前 admin id，用于 self-protection（按钮禁用 + 后端兜底）。
const currentAdminId = computed<number>(() => adminInfo.id)

const searchKeyword = ref<string>('')
const page = ref<number>(1)
const pageSize = ref<number>(20)
const total = ref<number>(0)
const items = ref<AdminInfo[]>([])
const loading = ref<boolean>(false)
const selectedIds = ref<number[]>([])

async function loadList() {
    loading.value = true
    try {
        const { data } = await adminList({
            page: page.value,
            page_size: pageSize.value,
            username: searchKeyword.value || undefined,
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

function onSelectionChange(rows: AdminInfo[]) {
    selectedIds.value = rows.map((r) => r.id)
}

// --- 通用 helpers ---

function isSelf(row: AdminInfo): boolean {
    return row.id === currentAdminId.value
}

function formatCreatedAt(id: number): string {
    // 后端 list 接口目前未返回 created_at；UI 暂显示 id 兜底，后续 task 可加。
    return String(id)
}

// --- 创建 / 编辑 弹窗 ---

const formDialogVisible = ref<boolean>(false)
const formDialogMode = ref<'create' | 'edit'>('create')
const submitting = ref<boolean>(false)
const formRef = ref<FormInstance>()

interface AdminForm {
    id?: number
    username: string
    nickname: string
    email: string
    mobile: string
    avatar: string
    password: string
    bio: string
    status: 0 | 1
}

const form = reactive<AdminForm>({
    username: '',
    nickname: '',
    email: '',
    mobile: '',
    avatar: '',
    password: '',
    bio: '',
    status: 1,
})

const formRules = reactive<FormRules<AdminForm>>({
    username: [
        { required: true, message: () => t('manager.columns.username'), trigger: 'blur' },
        { min: 3, message: 'min 3', trigger: 'blur' },
    ],
    nickname: [{ required: false }],
    password: [
        {
            validator: (_rule, value, cb) => {
                if (formDialogMode.value === 'create' && (!value || value.length < 8)) {
                    cb(new Error('min 8'))
                } else {
                    cb()
                }
            },
            trigger: 'blur',
        },
    ],
})

function resetForm() {
    form.id = undefined
    form.username = ''
    form.nickname = ''
    form.email = ''
    form.mobile = ''
    form.avatar = ''
    form.password = ''
    form.bio = ''
    form.status = 1
}

function openCreateDialog() {
    resetForm()
    formDialogMode.value = 'create'
    formDialogVisible.value = true
}

function openEditDialog(row: AdminInfo) {
    resetForm()
    formDialogMode.value = 'edit'
    form.id = row.id
    form.username = row.username
    form.nickname = row.nickname
    form.email = row.email ?? ''
    form.mobile = row.mobile ?? ''
    form.avatar = row.avatar ?? ''
    form.bio = row.bio ?? ''
    form.status = row.status
    formDialogVisible.value = true
}

async function submitForm() {
    if (!formRef.value) return
    await formRef.value.validate()
    submitting.value = true
    try {
        if (formDialogMode.value === 'create') {
            await adminCreate({
                username: form.username,
                nickname: form.nickname,
                email: form.email || undefined,
                mobile: form.mobile || undefined,
                avatar: form.avatar || undefined,
                password: form.password,
                bio: form.bio,
                status: form.status,
            })
            ElMessage.success(t('manager.dialog.create'))
        } else {
            await adminEdit({
                id: form.id,
                username: form.username,
                nickname: form.nickname,
                email: form.email || undefined,
                mobile: form.mobile || undefined,
                avatar: form.avatar || undefined,
                bio: form.bio,
                status: form.status,
            })
            ElMessage.success(t('manager.dialog.edit'))
        }
        formDialogVisible.value = false
        loadList()
    } finally {
        submitting.value = false
    }
}

// --- 改密 弹窗 ---

const passwordDialogVisible = ref<boolean>(false)
const passwordFormRef = ref<FormInstance>()
const passwordTarget = ref<AdminInfo | null>(null)
const passwordForm = reactive<{ new_password: string }>({ new_password: '' })
const passwordRules = reactive<FormRules<{ new_password: string }>>({
    new_password: [{ required: true, min: 8, message: 'min 8', trigger: 'blur' }],
})

function resetPasswordForm() {
    passwordForm.new_password = ''
    passwordTarget.value = null
}

function openChangePasswordDialog(row: AdminInfo) {
    resetPasswordForm()
    passwordTarget.value = row
    passwordDialogVisible.value = true
}

async function submitPassword() {
    if (!passwordFormRef.value) return
    await passwordFormRef.value.validate()
    submitting.value = true
    try {
        await adminChangePassword({
            id: passwordTarget.value!.id,
            new_password: passwordForm.new_password,
        })
        ElMessage.success(t('manager.dialog.changePassword'))
        passwordDialogVisible.value = false
    } finally {
        submitting.value = false
    }
}

// --- 行内操作 ---

async function handleToggleStatus(row: AdminInfo) {
    await adminToggleStatus({ id: row.id })
    ElMessage.success(t('manager.action.toggleStatus'))
    loadList()
}

async function handleUnlock(row: AdminInfo) {
    await adminUnlock({ id: row.id })
    ElMessage.success(t('manager.action.unlock'))
    loadList()
}

async function handleDelete(row: AdminInfo) {
    await ElMessageBox.confirm(t('manager.confirm.delete'), '', { type: 'warning' })
    await adminDelete(row.id)
    ElMessage.success(t('manager.action.delete'))
    loadList()
}

async function handleBatchDelete() {
    if (selectedIds.value.length === 0) return
    await ElMessageBox.confirm(t('manager.confirm.batchDelete'), '', { type: 'warning' })
    const { data } = await adminBatchDelete({ ids: selectedIds.value })
    ElMessage.success(t('manager.action.batchDelete') + `: deleted=${data.deleted}, skipped_self=${data.skipped_self}`)
    loadList()
}

onMounted(() => {
    loadList()
})
</script>

<style scoped lang="scss">
.admin-manager {
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
