<template>
    <div class="aginput-test">
        <el-alert
            type="warning"
            :closable="false"
            show-icon
            title="后端 /admin/ajax/upload 端点尚未实现"
            description="agUpload 组件已就绪，调用 fileUpload 时会得到 404（属预期）。此处用于校验前端组件渲染与事件链路。"
        />

        <el-row :gutter="16" class="mt-16">
            <el-col :xs="24" :md="12">
                <el-card shadow="hover" class="mb-16">
                    <template #header>
                        <span class="card-title">单图（type=image）</span>
                    </template>
                    <ag-upload type="image" v-model="singleImage" :return-full-url="true" />
                    <el-input class="mt-12" type="textarea" :rows="2" :model-value="singleImage" readonly placeholder="v-model 当前值" />
                </el-card>
            </el-col>

            <el-col :xs="24" :md="12">
                <el-card shadow="hover" class="mb-16">
                    <template #header>
                        <span class="card-title">多图（type=images，limit=6）</span>
                    </template>
                    <ag-upload type="images" v-model="multiImages" :return-full-url="true" :limit="6" />
                    <el-input class="mt-12" type="textarea" :rows="3" :model-value="multiImages.join(',')" readonly placeholder="v-model 当前值" />
                </el-card>
            </el-col>

            <el-col :xs="24" :md="12">
                <el-card shadow="hover" class="mb-16">
                    <template #header>
                        <span class="card-title">单文件（type=file）</span>
                    </template>
                    <ag-upload type="file" v-model="singleFile" />
                    <el-input class="mt-12" type="textarea" :rows="2" :model-value="singleFile" readonly placeholder="v-model 当前值" />
                </el-card>
            </el-col>

            <el-col :xs="24" :md="12">
                <el-card shadow="hover" class="mb-16">
                    <template #header>
                        <span class="card-title">多文件（type=files，limit=6）</span>
                    </template>
                    <ag-upload type="files" v-model="multiFiles" :limit="6" />
                    <el-input class="mt-12" type="textarea" :rows="3" :model-value="multiFiles.join(',')" readonly placeholder="v-model 当前值" />
                </el-card>
            </el-col>
        </el-row>
    </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import agUpload from '/@/components/agInput/components/agUpload.vue'

defineOptions({
    name: 'agInput',
})

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
</style>
