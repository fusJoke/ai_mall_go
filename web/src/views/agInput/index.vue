<template>
    <div class="aginput-test">
        <el-alert
            type="info"
            :closable="false"
            show-icon
            title="多驱动上传测试"
            description="上传请求携带 driver 参数（local / aliyun / tencent / qiniu），实际驱动由服务端 POST /admin/ajax/upload 决定；对象存储驱动当前为示例数据，服务端未实现时会回落本地存储。"
        />

        <!-- 多驱动选择 -->
        <el-card shadow="hover" class="mt-16 mb-16">
            <template #header>
                <span class="card-title">上传驱动（示例数据）</span>
            </template>
            <el-row :gutter="12">
                <el-col v-for="driver in drivers" :key="driver.value" :xs="12" :md="6">
                    <div class="driver-card" :class="{ active: selectedDriver === driver.value }" @click="selectedDriver = driver.value">
                        <div class="driver-name">
                            {{ driver.name }}
                            <el-tag v-if="driver.isDefault" size="small" type="success">默认</el-tag>
                            <el-tag v-else-if="!driver.configured" size="small" type="info">示例</el-tag>
                        </div>
                        <div class="driver-desc">{{ driver.description }}</div>
                    </div>
                </el-col>
            </el-row>
            <div class="driver-current mt-12">
                当前驱动：
                <el-tag :type="selectedDriver === 'local' ? 'success' : 'warning'">{{ selectedDriver }}</el-tag>
                <span class="driver-hint">（请求参数：?driver={{ selectedDriver }}）</span>
            </div>
        </el-card>

        <el-row :gutter="16" class="mt-16">
            <el-col :xs="24" :md="12">
                <el-card shadow="hover" class="mb-16">
                    <template #header>
                        <span class="card-title">单图（type=image）</span>
                    </template>
                    <ag-upload type="image" v-model="singleImage" :return-full-url="true" :driver="selectedDriver" />
                    <el-input class="mt-12" type="textarea" :rows="2" :model-value="singleImage" readonly placeholder="v-model 当前值" />
                </el-card>
            </el-col>

            <el-col :xs="24" :md="12">
                <el-card shadow="hover" class="mb-16">
                    <template #header>
                        <span class="card-title">多图（type=images，limit=6）</span>
                    </template>
                    <ag-upload type="images" v-model="multiImages" :return-full-url="true" :limit="6" :driver="selectedDriver" />
                    <el-input class="mt-12" type="textarea" :rows="3" :model-value="multiImages.join(',')" readonly placeholder="v-model 当前值" />
                </el-card>
            </el-col>

            <el-col :xs="24" :md="12">
                <el-card shadow="hover" class="mb-16">
                    <template #header>
                        <span class="card-title">单文件（type=file）</span>
                    </template>
                    <ag-upload type="file" v-model="singleFile" :driver="selectedDriver" />
                    <el-input class="mt-12" type="textarea" :rows="2" :model-value="singleFile" readonly placeholder="v-model 当前值" />
                </el-card>
            </el-col>

            <el-col :xs="24" :md="12">
                <el-card shadow="hover" class="mb-16">
                    <template #header>
                        <span class="card-title">多文件（type=files，limit=6）</span>
                    </template>
                    <ag-upload type="files" v-model="multiFiles" :limit="6" :driver="selectedDriver" />
                    <el-input class="mt-12" type="textarea" :rows="3" :model-value="multiFiles.join(',')" readonly placeholder="v-model 当前值" />
                </el-card>
            </el-col>
        </el-row>
    </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import agUpload from '/@/components/agInput/components/agUpload.vue'
import { exampleUploadDrivers, type UploadDriverValue } from '/@/mock/uploadDrivers'

defineOptions({
    name: 'agInput',
})

/** 上传驱动示例数据（mock/uploadDrivers.ts，服务端接入后由后端下发） */
const drivers = exampleUploadDrivers

const selectedDriver = ref<UploadDriverValue>('local')

const singleImage = ref<string>('')
const multiImages = ref<string[]>([])
const singleFile = ref<string>('')
const multiFiles = ref<string[]>([])
</script>

<style scoped lang="scss">
.aginput-test {
    padding: 16px;
}
.card-title {
    font-weight: 600;
}
.mb-16 {
    margin-bottom: 16px;
}
.mt-12 {
    margin-top: 12px;
}
.mt-16 {
    margin-top: 16px;
}
.driver-card {
    height: 100%;
    padding: 12px;
    border: 1px solid var(--el-border-color-lighter);
    border-radius: var(--el-border-radius-base);
    cursor: pointer;
    transition: all 0.2s;

    &:hover {
        border-color: var(--el-color-primary-light-5);
    }
    &.active {
        border-color: var(--el-color-primary);
        box-shadow: 0 0 0 1px var(--el-color-primary);
    }

    .driver-name {
        display: flex;
        align-items: center;
        gap: 6px;
        font-weight: 600;
        margin-bottom: 6px;
    }
    .driver-desc {
        font-size: 12px;
        color: var(--el-text-color-secondary);
    }
}
.driver-current {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 13px;

    .driver-hint {
        color: var(--el-text-color-secondary);
    }
}
</style>
