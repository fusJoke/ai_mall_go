<template>
    <div
        ref="wrapper"
        class="w100 ag-upload-wrapper"
        :class="{ 'is-drag-over': state.isDragOver, 'is-disabled': state.attrs.disabled }"
        tabindex="0"
        @paste="onPaste"
    >
        <el-upload
            ref="upload"
            class="ag-upload"
            :class="[
                type,
                state.attrs.disabled ? 'is-disabled' : '',
                hideImagePlusOnOverLimit && state.attrs.limit && state.fileList.length >= state.attrs.limit ? 'hide-image-plus' : '',
            ]"
            v-model:file-list="state.fileList"
            :auto-upload="false"
            @change="onElChange"
            @remove="onElRemove"
            @preview="onElPreview"
            @exceed="onElExceed"
            v-bind="state.attrs"
            :key="state.key"
        >
            <template v-if="!$slots.default" #default>
                <template v-if="type == 'image' || type == 'images'">
                    <el-tooltip :content="pasteUploadTip" placement="top" :disabled="!!state.attrs.disabled || state.isDragOver">
                        <div
                            class="ag-upload-trigger"
                            @mouseenter="onTriggerFocus"
                            @dragenter.prevent="onDragEnter"
                            @dragover.prevent="onDragOver"
                            @dragleave.prevent="onDragLeave"
                            @drop.prevent="onDrop"
                        >
                            <div v-if="!hideSelectFile" @click.stop="showSelectFile()" class="ag-upload-select-image">
                                {{ t('utils.choice') }}
                            </div>
                            <Icon class="ag-upload-icon" name="el-icon-Plus" size="30" color="#c0c4cc" />
                            <div @click.stop="onPasteUpload()" class="ag-upload-paste-image">
                                {{ t('utils.screenshotUpload') }}
                            </div>
                            <div v-if="state.isDragOver && !state.attrs.disabled" class="ag-upload-drag-mask">
                                <Icon name="el-icon-UploadFilled" size="40" color="var(--el-color-primary)" />
                                <span>{{ dragDropTip }}</span>
                            </div>
                        </div>
                    </el-tooltip>
                </template>
                <template v-else>
                    <el-tooltip :content="pasteUploadTip" placement="top" :disabled="!!state.attrs.disabled || state.isDragOver">
                        <div
                            class="ag-upload-trigger ag-upload-trigger-file"
                            @mouseenter="onTriggerFocus"
                            @dragenter.prevent="onDragEnter"
                            @dragover.prevent="onDragOver"
                            @dragleave.prevent="onDragLeave"
                            @drop.prevent="onDrop"
                        >
                            <el-button v-blur type="primary">
                                <Icon name="el-icon-Plus" color="#ffffff" />
                                <span>{{ t('Upload') }}</span>
                            </el-button>
                            <el-button v-blur v-if="!hideSelectFile" @click.stop="showSelectFile()" type="success">
                                <Icon name="fa fa-th-list" size="14px" color="#ffffff" />
                                <span class="ml-6">{{ t('utils.choice') }}</span>
                            </el-button>
                            <div v-if="state.isDragOver && !state.attrs.disabled" class="ag-upload-drag-mask">
                                <Icon name="el-icon-UploadFilled" size="40" color="var(--el-color-primary)" />
                                <span>{{ dragDropTip }}</span>
                            </div>
                        </div>
                    </el-tooltip>
                </template>
            </template>

            <template v-for="(slot, name) in $slots" #[name]="scopedData">
                <slot :name="name" v-bind="scopedData"></slot>
            </template>
        </el-upload>
        <el-dialog v-model="state.preview.show" :append-to-body="true" :destroy-on-close="true" class="ag-upload-preview">
            <div class="ag-upload-preview-scroll">
                <img :src="state.preview.url" class="ag-upload-preview-img" alt="" />
            </div>
        </el-dialog>
        <SelectFile v-model="state.selectFile.show" v-bind="state.selectFile" @choice="onChoice" />
    </div>
</template>

<script setup lang="ts">
import { reactive, onMounted, watch, useAttrs, nextTick, useTemplateRef, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { genFileId, ElMessage } from 'element-plus'
import type { UploadUserFile, UploadProps, UploadRawFile, UploadFiles } from 'element-plus'
import { fullURL as fullUrl, fullURLArray as arrayFullUrl, getFileNameFromPath, getArrayKey, stringToArray } from '/@/utils/common'
import { fileUpload } from '/@/api/common'
import SelectFile from '/@/components/agInput/components/selectFile.vue'
import { uuid } from '/@/utils/random'
import { cloneDeep, isEmpty } from 'lodash-es'
import type { AxiosProgressEvent } from 'axios'
import Sortable from 'sortablejs'
import Icon from '/@/components/icon/index.vue'

// 禁用 Attributes 自动继承
defineOptions({
    inheritAttrs: false,
})

interface Props extends /* @vue-ignore */ Partial<UploadProps> {
    type: 'image' | 'images' | 'file' | 'files'
    // 上传请求时的额外携带数据
    data?: anyObj
    modelValue: string | string[]
    // 返回绝对路径
    returnFullUrl?: boolean
    // 隐藏附件选择器
    hideSelectFile?: boolean
    // 强制上传到本地存储
    forceLocal?: boolean
    // 上传驱动（local / aliyun / tencent / qiniu...），随请求发送 driver 参数，
    // 实际驱动由服务端决定（多驱动上传约定，见 mock/uploadDrivers.ts）
    driver?: string
    // 在上传数量达到限制时隐藏图片上传按钮
    hideImagePlusOnOverLimit?: boolean
}
interface UploadFileExt extends UploadUserFile {
    serverUrl?: string
}
interface UploadProgressEvent extends AxiosProgressEvent {
    percent: number
}

const props = withDefaults(defineProps<Props>(), {
    type: 'image',
    data: () => {
        return {}
    },
    modelValue: () => [],
    returnFullUrl: false,
    hideSelectFile: false,
    forceLocal: false,
    driver: 'local',
    hideImagePlusOnOverLimit: false,
})

const emits = defineEmits<{
    (e: 'update:modelValue', value: string | string[]): void
}>()

const { t } = useI18n()
const attrs = useAttrs()
const upload = useTemplateRef('upload')
const wrapper = useTemplateRef('wrapper')

const isImageType = () => props.type == 'image' || props.type == 'images'

const dragDropTip = computed(() => {
    return isImageType() ? t('utils.dropImageHereToUpload') : t('utils.dropFileHereToUpload')
})

const pasteUploadTip = computed(() => t('utils.copyPasteUploadTip'))

/**
 * 将文件加入 el-upload 并开始上传
 */
const startUploadFile = (rawFile: UploadRawFile) => {
    rawFile.uid = genFileId()
    if (state.attrs.limit && state.fileList.length >= state.attrs.limit) {
        onElExceed([rawFile])
    } else {
        upload.value!.handleStart(rawFile)
    }
}

/**
 * 从拖拽数据中收集可上传的文件
 */
const collectDropFiles = (fileList: FileList | File[]) => {
    const files = Array.from(fileList)
    if (!files.length) return []

    if (isImageType()) {
        return files.filter((file) => file.type.startsWith('image/'))
    }
    return files
}

/**
 * 批量上传文件
 */
const uploadFiles = (validFiles: File[]) => {
    if (!validFiles.length) return false

    const limit = state.attrs.limit
    let remaining = limit ? Math.max(limit - state.fileList.length, 0) : validFiles.length

    for (const file of validFiles) {
        if (limit && remaining <= 0) {
            if (limit === 1) {
                startUploadFile(file as UploadRawFile)
            }
            break
        }
        startUploadFile(file as UploadRawFile)
        if (limit) remaining--
    }
    return true
}

/**
 * 从粘贴板事件中收集可上传的文件
 */
const collectPasteFiles = (event: ClipboardEvent) => {
    const files: File[] = []
    const clipboardData = event.clipboardData
    if (!clipboardData) return []

    if (clipboardData.files.length) {
        files.push(...Array.from(clipboardData.files))
    } else {
        Array.from(clipboardData.items).forEach((item) => {
            if (item.kind === 'file') {
                const file = item.getAsFile()
                if (file) files.push(file)
            }
        })
    }
    return collectDropFiles(files)
}

const onTriggerFocus = () => {
    if (state.attrs.disabled) return
    wrapper.value?.focus({ preventScroll: true })
}

const onPaste = (event: ClipboardEvent) => {
    if (state.attrs.disabled) return

    const validFiles = collectPasteFiles(event)
    if (!validFiles.length) {
        ElMessage.warning(isImageType() ? t('utils.noImageInClipboard') : t('utils.noValidFileInPaste'))
        return
    }

    event.preventDefault()
    uploadFiles(validFiles)
}

const resetDragState = () => {
    state.isDragOver = false
    state.dragCounter = 0
}

const onDragEnter = () => {
    if (state.attrs.disabled) return
    state.dragCounter++
    state.isDragOver = true
}

const onDragOver = () => {
    if (state.attrs.disabled) return
    state.isDragOver = true
}

const onDragLeave = () => {
    if (state.attrs.disabled) return
    state.dragCounter--
    if (state.dragCounter <= 0) {
        resetDragState()
    }
}

const onDrop = (event: DragEvent) => {
    resetDragState()
    if (state.attrs.disabled) return

    const validFiles = collectDropFiles(event.dataTransfer?.files || [])
    if (!uploadFiles(validFiles)) {
        ElMessage.warning(isImageType() ? t('utils.noValidImageInDrop') : t('utils.noValidFileInDrop'))
    }
}

const state: {
    key: string
    // 返回值类型，通过v-model类型动态计算
    defaultReturnType: 'string' | 'array'
    // 预览弹窗
    preview: {
        show: boolean
        url: string
    }
    // 文件列表
    fileList: UploadFileExt[]
    // 绑定到 el-upload 的属性对象
    attrs: Partial<UploadProps>
    // 正在上传的文件数量
    uploading: number
    // 显示选择文件窗口
    selectFile: {
        show: boolean
        type?: 'image' | 'file'
        limit?: number
        returnFullUrl: boolean
    }
    events: anyObj
    isDragOver: boolean
    dragCounter: number
} = reactive({
    key: uuid(),
    defaultReturnType: 'string',
    preview: {
        show: false,
        url: '',
    },
    fileList: [],
    attrs: {},
    uploading: 0,
    selectFile: {
        show: false,
        type: 'file',
        returnFullUrl: props.returnFullUrl,
    },
    events: {},
    isDragOver: false,
    dragCounter: 0,
})

/**
 * 需要管理的事件列表（使用 triggerEvent 触发）
 */
const eventNameMap = {
    // el-upload 的钩子函数（它们是 props，并不是 emit，以上已经使用，所以需要手动触发）
    change: ['onChange', 'on-change'],
    remove: ['onRemove', 'on-remove'],
    preview: ['onPreview', 'on-preview'],
    exceed: ['onExceed', 'on-exceed'],

    // 由于自定义了上传方法，需要手动触发的钩子
    beforeUpload: ['beforeUpload', 'onBeforeUpload', 'before-upload', 'on-before-upload'],
    progress: ['onProgress', 'on-progress'],
    success: ['onSuccess', 'on-success'],
    error: ['onError', 'on-error'],
}

const onElChange = (file: UploadFileExt, files: UploadFiles) => {
    // 将 file 换为 files 中的对象，以便修改属性等操作
    const fileIndex = getArrayKey(files, 'uid', file.uid!)
    if (fileIndex === false) return

    file = files[fileIndex] as UploadFileExt
    if (!file || !file.raw) return
    if (triggerEvent('beforeUpload', [file]) === false) return
    let fd = new FormData()
    fd.append('file', file.raw)
    fd = formDataAppend(fd)

    file.status = 'uploading'
    state.uploading++
    fileUpload(
        fd,
        { uuid: uuid(), force_local: props.forceLocal ? 1 : 0, driver: props.driver },
        {
            onUploadProgress: (evt: AxiosProgressEvent) => {
                const progressEvt = evt as UploadProgressEvent
                if (evt.total && evt.total > 0 && ['ready', 'uploading'].includes(file.status!)) {
                    progressEvt.percent = (evt.loaded / evt.total) * 100
                    file.status = 'uploading'
                    file.percentage = Math.round(progressEvt.percent)
                    triggerEvent('progress', [progressEvt, file, files])
                }
            },
        }
    )
        .then((res: any) => {
            if (res.code == 1) {
                file.serverUrl = res.data.file.url
                file.status = 'success'
                emits('update:modelValue', getAllUrls())
                triggerEvent('success', [res, file, files])
            } else {
                file.status = 'fail'
                files.splice(fileIndex, 1)
                triggerEvent('error', [res, file, files])
            }
        })
        .catch((res) => {
            file.status = 'fail'
            files.splice(fileIndex, 1)
            triggerEvent('error', [res, file, files])
        })
        .finally(() => {
            state.uploading--
            onChange(file, files)
        })
}

const onElRemove = (file: UploadUserFile, files: UploadFiles) => {
    triggerEvent('remove', [file, files])
    onChange(file, files)
    nextTick(() => {
        emits('update:modelValue', getAllUrls())
    })
}

const onElPreview = (file: UploadFileExt) => {
    triggerEvent('preview', [file])
    if (!file || !file.serverUrl) {
        return
    }
    if (props.type == 'file' || props.type == 'files') {
        window.open(fullUrl(file.serverUrl))
        return
    }
    state.preview.show = true
    state.preview.url = fullUrl(file.serverUrl)
}

const onElExceed = (files: UploadUserFile[]) => {
    const file = files[0] as UploadRawFile
    file.uid = genFileId()
    upload.value!.handleStart(file)
    triggerEvent('exceed', [file, state.fileList])
}

const onChoice = (files: string[]) => {
    let oldValArr = getAllUrls('array') as string[]
    files = oldValArr.concat(files)
    init(files)
    emits('update:modelValue', getAllUrls())
    onChange(files, state.fileList)
    state.selectFile.show = false
}

/**
 * 初始化文件/图片的排序功能
 */
const initSort = () => {
    if (state.attrs.showFileList === false) {
        return false
    }
    nextTick(() => {
        let uploadListEl = upload.value?.$el.querySelector('.el-upload-list')
        let uploadItemEl = uploadListEl.getElementsByClassName('el-upload-list__item')
        if (uploadItemEl.length >= 2) {
            Sortable.create(uploadListEl, {
                animation: 200,
                draggable: '.el-upload-list__item',
                onEnd: (evt: Sortable.SortableEvent) => {
                    if (evt.oldIndex != evt.newIndex) {
                        state.fileList[evt.newIndex!] = [
                            state.fileList[evt.oldIndex!],
                            (state.fileList[evt.oldIndex!] = state.fileList[evt.newIndex!]),
                        ][0]
                        emits('update:modelValue', getAllUrls())
                    }
                },
            })
        }
    })
}

const triggerEvent = (name: string, args: any[]) => {
    const events = eventNameMap[name as keyof typeof eventNameMap]
    if (events) {
        for (const key in events) {
            // 执行函数，只在 false 时 return
            if (typeof state.events[events[key]] === 'function' && state.events[events[key]](...args) === false) return false
        }
    }
}

onMounted(() => {
    let events: string[] = []
    let bindAttrs: anyObj = {}
    for (const key in eventNameMap) {
        events = [...events, ...eventNameMap[key as keyof typeof eventNameMap]]
    }
    for (const attrKey in attrs) {
        if (events.includes(attrKey)) {
            state.events[attrKey] = attrs[attrKey]
        } else {
            bindAttrs[attrKey] = attrs[attrKey]
        }
    }

    if (props.type == 'image' || props.type == 'file') {
        bindAttrs = { ...bindAttrs, limit: 1 }
    } else {
        bindAttrs = { ...bindAttrs, multiple: true }
    }

    if (props.type == 'image' || props.type == 'images') {
        state.selectFile.type = 'image'
        bindAttrs = { ...bindAttrs, accept: 'image/*', listType: 'picture-card' }
    }

    state.attrs = { ...bindAttrs }

    // 设置附件选择器的 limit
    if (state.attrs.limit) {
        state.selectFile.limit = state.attrs.limit
    }

    init(props.modelValue)

    initSort()
})

const limitExceed = () => {
    if (state.attrs.limit && state.fileList.length > state.attrs.limit) {
        state.fileList = state.fileList.slice(state.fileList.length - state.attrs.limit)
        return true
    }
    return false
}

const init = (modelValue: string | string[]) => {
    let urls = stringToArray(modelValue as string)
    state.fileList = []
    state.defaultReturnType = typeof modelValue === 'string' || props.type == 'file' || props.type == 'image' ? 'string' : 'array'

    for (const key in urls) {
        state.fileList.push({
            name: getFileNameFromPath(urls[key]),
            url: fullUrl(urls[key]),
            serverUrl: urls[key],
        })
    }

    // 超出过滤 || 确定返回的URL完整
    if (limitExceed() || props.returnFullUrl) {
        emits('update:modelValue', getAllUrls())
    }
    state.key = uuid()
}

/**
 * 获取当前所有图片路径的列表
 */
const getAllUrls = (returnType: string = state.defaultReturnType) => {
    limitExceed()
    let urlList = []
    for (const key in state.fileList) {
        if (state.fileList[key].serverUrl) urlList.push(state.fileList[key].serverUrl)
    }
    if (props.returnFullUrl) urlList = arrayFullUrl(urlList as string[])
    return returnType === 'string' ? urlList.join(',') : (urlList as string[])
}

const formDataAppend = (fd: FormData) => {
    if (props.data && !isEmpty(props.data)) {
        for (const key in props.data) {
            fd.append(key, props.data[key])
        }
    }
    return fd
}

/**
 * 文件状态改变时的钩子，选择文件、上传成功和上传失败时都会被调用
 */
const onChange = (file: string | string[] | UploadFileExt, files: UploadFileExt[]) => {
    initSort()
    triggerEvent('change', [file, files])
}

const getRef = () => {
    return upload.value
}

const showSelectFile = () => {
    if (state.attrs.disabled) return
    state.selectFile.show = true
}

/**
 * 从粘贴板读取图片并上传
 */
const onPasteUpload = async () => {
    if (state.attrs.disabled) return

    if (!navigator.clipboard?.read) {
        ElMessage.warning(t('utils.clipboardReadNotSupported'))
        return
    }

    try {
        const clipboardItems = await navigator.clipboard.read()
        let imageBlob: Blob | null = null
        let mimeType = ''

        for (const item of clipboardItems) {
            const imageType = item.types.find((type) => type.startsWith('image/'))
            if (imageType) {
                imageBlob = await item.getType(imageType)
                mimeType = imageType
                break
            }
        }

        if (!imageBlob) {
            ElMessage.warning(t('utils.noImageInClipboard'))
            return
        }

        const ext = mimeType.split('/')[1]?.replace('jpeg', 'jpg') || 'png'
        const file = new File([imageBlob], `paste-${Date.now()}.${ext}`, { type: mimeType }) as UploadRawFile
        startUploadFile(file)
    } catch {
        ElMessage.warning(t('utils.failedToReadClipboard'))
    }
}

defineExpose({
    getRef,
    showSelectFile,
})

watch(
    () => props.modelValue,
    (newVal) => {
        if (state.uploading > 0) return
        if (newVal === undefined || newVal === null) {
            return init('')
        }
        let newValArr = arrayFullUrl(stringToArray(cloneDeep(newVal)))
        let oldValArr = arrayFullUrl(getAllUrls('array') as string[])
        if (newValArr.sort().toString() != oldValArr.sort().toString()) {
            init(newVal)
        }
    }
)
</script>

<style scoped lang="scss">
.ag-upload-wrapper {
    position: relative;
    outline: none;
    &.is-drag-over:not(.is-disabled) :deep(.el-upload--picture-card),
    &.is-drag-over:not(.is-disabled) :deep(.el-upload-dragger) {
        border-color: var(--el-color-primary);
    }
}
.ag-upload-trigger {
    position: absolute;
    inset: 0;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 100%;
    height: 100%;
    &.ag-upload-trigger-file {
        position: relative;
        inset: unset;
        gap: 6px;
        padding: 0 10px;
        min-height: 32px;
    }
}
.ag-upload-drag-mask {
    position: absolute;
    inset: 0;
    z-index: 10;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 8px;
    // 拖拽遮罩：白底半透明 + 虚线边框。
    // `background` / `border` 这些 CSS 关键字含 "ba" 子串，会被原始 grep `ba-?` 当成
    // buildadmin 命名空间残留的误报。spec 5.3 已改为 `\bba-[a-zA-Z]` 精确匹配。
    background-color: rgba(255, 255, 255, 0.92);
    border: 2px dashed var(--el-color-primary);
    border-radius: 6px;
    color: var(--el-color-primary);
    font-size: var(--el-font-size-small);
    pointer-events: none;
}
.ag-upload-select-image,
.ag-upload-paste-image {
    position: absolute;
    border: 1px dashed var(--el-border-color);
    width: var(--el-upload-picture-card-size);
    height: 30px;
    line-height: 30px;
    border-radius: 6px;
    text-align: center;
    font-size: var(--el-font-size-extra-small);
    color: var(--el-text-color-regular);
    user-select: none;
    &:hover {
        color: var(--el-color-primary);
        border: 1px dashed var(--el-color-primary);
    }
}
.ag-upload-select-image {
    top: 0px;
    border-top: 1px dashed transparent;
    border-bottom-right-radius: 20px;
    border-bottom-left-radius: 20px;
    &:hover {
        border-top: 1px dashed var(--el-color-primary);
    }
}
.ag-upload-paste-image {
    bottom: 0px;
    border-bottom: 1px dashed transparent;
    border-top-right-radius: 20px;
    border-top-left-radius: 20px;
    &:hover {
        border-bottom: 1px dashed var(--el-color-primary);
    }
}
.ag-upload :deep(.el-upload:hover .ag-upload-icon) {
    color: var(--el-color-primary) !important;
}
:deep(.ag-upload-preview) .el-dialog__body {
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 10px;
    height: auto;
}
.ag-upload-preview-scroll {
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 10px;
    height: auto;
    overflow: auto;
    max-height: 70vh;
}
.ag-upload-preview-img {
    max-width: 100%;
    max-height: 100%;
}
:deep(.el-dialog__headerbtn) {
    top: 2px;
    width: 37px;
    height: 37px;
}
.ag-upload.image :deep(.el-upload--picture-card),
.ag-upload.images :deep(.el-upload--picture-card) {
    position: relative;
    display: inline-flex;
    align-items: center;
    justify-content: center;
}
.ag-upload.image :deep(.el-upload--picture-card > .el-tooltip__trigger),
.ag-upload.images :deep(.el-upload--picture-card > .el-tooltip__trigger) {
    position: absolute;
    inset: 0;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 100%;
    height: 100%;
}
.ag-upload.file :deep(.el-upload),
.ag-upload.files :deep(.el-upload) {
    position: relative;
}
.ag-upload.file :deep(.el-upload-list),
.ag-upload.files :deep(.el-upload-list) {
    margin-left: -10px;
}
.ag-upload.files,
.ag-upload.images {
    :deep(.el-upload-list__item) {
        user-select: none;
        .el-upload-list__item-actions,
        .el-upload-list__item-name {
            cursor: move;
        }
    }
}
.ml-6 {
    margin-left: 6px;
}
.ag-upload.hide-image-plus :deep(.el-upload--picture-card) {
    display: none;
}
.ag-upload.is-disabled :deep(.el-upload),
.ag-upload.is-disabled :deep(.el-upload) .el-button,
.ag-upload.is-disabled :deep(.el-upload--picture-card) {
    cursor: not-allowed;
}
</style>
