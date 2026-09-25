<template>
    <div class="loading">
        <span class="loading-text">加载中...</span>
    </div>
</template>

<script setup lang="ts"></script>

<style scoped lang="scss">
.loading {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 100%;
    height: 100%;
    min-height: 200px;
}
.loading-text {
    color: var(--el-text-color-secondary);
    font-size: var(--el-font-size-base);
}
</style>