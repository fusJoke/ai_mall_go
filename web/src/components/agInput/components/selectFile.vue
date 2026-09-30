<template>
    <el-dialog
        @close="emits('update:modelValue', false)"
        :model-value="modelValue"
        class="ag-upload-select-dialog"
        :title="t('utils.selectFile')"
        :append-to-body="true"
        :destroy-on-close="true"
        top="4vh"
        width="60%"
    >
        <div v-if="list && list.length" class="ag-upload-select-list">
            <el-table ref="tableRef" :data="list" @selection-change="onSelectionChange" max-height="500" border stripe>
                <el-table-column type="selection" width="55" :selectable="isRowSelectable" :reserve-selection="false" />
                <el-table-column :label="t('utils.originalName')" prop="name" show-overflow-tooltip />
                <el-table-column :label="t('utils.url')" prop="url" show-overflow-tooltip>
                    <template #default="{ row }">
                        <a v-if="row.url" :href="row.url" target="_blank" rel="noopener">{{ row.url }}</a>
                    </template>
                </el-table-column>
                <el-table-column :label="t('utils.operate')" align="center" width="120">
                    <template #default="{ row }">
                        <el-button size="small" type="primary" @click="onChoiceOne(row)">
                            <Icon name="fa fa-check" />
                            <span class="ml-6">{{ t('utils.choice') }}</span>
                        </el-button>
                    </template>
                </el-table-column>
            </el-table>
        </div>
        <el-empty v-else :description="t('utils.noData')" />
        <template #footer>
            <div class="ag-upload-select-footer">
                <span v-if="limit !== 0" class="footer-tip">
                    {{ t('utils.youCanAlsoSelect') }}
                    <span class="selection-count">{{ Math.max((limit || 0) - selection.length, 0) }}</span>
                    {{ t('utils.items') }}
                </span>
                <el-button @click="onChoice" :disabled="!selection.length" type="primary" v-blur>
                    <Icon name="fa fa-check" />
                    <span class="ml-6">{{ t('utils.choice') }}</span>
                </el-button>
            </div>
        </template>
    </el-dialog>
</template>

<script setup lang="ts">
import { ref, watch, nextTick, useTemplateRef } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '/@/components/icon/index.vue'

export interface SelectFileItem {
    /** 后端存的相对 / 绝对 URL，最终通过 fullURL 转成对外可用地址 */
    url: string
    /** 原始文件名（用于展示） */
    name: string
}

interface Props {
    /** 列表数据 */
    list?: SelectFileItem[]
    /** 限定选择类型（image / file），只影响提示，不做强校验 */
    type?: 'image' | 'file'
    /** 最多可选数量，0 表示不限制 */
    limit?: number
    modelValue: boolean
    /** 当 true 时 emits 的 url 走 fullURL 转绝对路径 */
    returnFullUrl?: boolean
}

const props = withDefaults(defineProps<Props>(), {
    list: () => [],
    type: 'file',
    limit: 0,
    modelValue: false,
    returnFullUrl: false,
})

const emits = defineEmits<{
    (e: 'update:modelValue', value: boolean): void
    (e: 'choice', value: string[]): void
}>()

const { t } = useI18n()
const tableRef = useTemplateRef('tableRef')
const selection = ref<SelectFileItem[]>([])

const onSelectionChange = (rows: SelectFileItem[]) => {
    selection.value = rows
}

const isRowSelectable = (row: SelectFileItem) => {
    if (props.limit === 0) return true
    if (selection.value.some((r) => r.url === row.url)) return true
    return selection.value.length < (props.limit || 0)
}

const onChoiceOne = (row: SelectFileItem) => {
    emits('choice', [row.url])
    emits('update:modelValue', false)
}

const onChoice = () => {
    if (!selection.value.length) return
    const files = selection.value.map((r) => r.url)
    emits('choice', files)
    emits('update:modelValue', false)
}

// 关闭后清空选择状态（下次再打开时是空表）
watch(
    () => props.modelValue,
    (newVal) => {
        if (!newVal) {
            selection.value = []
            nextTick(() => {
                const elTableRef = tableRef.value as any
                elTableRef?.clearSelection?.()
            })
        }
    }
)
</script>

<style scoped lang="scss">
.ag-upload-select-dialog {
    :deep(.el-dialog__body) {
        padding: 10px 20px;
    }
}
.ag-upload-select-list {
    width: 100%;
}
.ag-upload-select-footer {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: 12px;
}
.footer-tip {
    margin-right: auto;
    color: var(--el-text-color-regular);
    font-size: var(--el-font-size-small);
}
.selection-count {
    color: var(--el-color-primary);
    font-weight: bold;
}
.ml-6 {
    margin-left: 6px;
}
</style>
