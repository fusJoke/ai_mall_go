<template>
    <div :title="$t('layouts.exitFullscreen')" @mouseover.stop="onMouseover" @mouseout.stop="onMouseout">
        <div class="close-full-screen" :style="{ top: state.closeBoxTop + 'px' }" @click.stop="onCloseFullScreen">
            <Icon name="lucide:X" size="16" />
        </div>
        <div class="close-full-screen-on"></div>
    </div>
</template>

<script setup lang="ts">
import { onMounted, reactive } from 'vue'
import { useNavTabs } from '/@/stores/navTabs'

/**
 * 标签页全屏态的关闭热区（对齐 buildadmin closeFullScreen.vue）：
 * 鼠标滑到顶部唤出关闭按钮，点击退出当前标签全屏。
 */
const navTabs = useNavTabs()

const state = reactive({
    closeBoxTop: 20,
})

onMounted(() => {
    setTimeout(() => {
        state.closeBoxTop = -30
    }, 300)
})

const onMouseover = () => {
    state.closeBoxTop = 20
}

const onMouseout = () => {
    state.closeBoxTop = -30
}

const onCloseFullScreen = () => {
    navTabs.setTabFullScreen(false)
}
</script>

<style scoped lang="scss">
.close-full-screen {
    display: flex;
    align-items: center;
    justify-content: center;
    position: fixed;
    right: calc(50% - 20px);
    z-index: 1000;
    height: 40px;
    width: 40px;
    background-color: rgba(0, 0, 0, 0.1);
    border-radius: 50%;
    box-shadow: var(--el-box-shadow-light);
    transition: all 0.3s ease;
    cursor: pointer;

    .icon {
        color: rgba(0, 0, 0, 0.6) !important;
    }

    &:hover {
        background-color: rgba(0, 0, 0, 0.3);
        .icon {
            color: rgba(255, 255, 255, 0.6) !important;
        }
    }
}
.close-full-screen-on {
    position: fixed;
    top: 0;
    z-index: 999;
    height: 60px;
    width: 100px;
    left: calc(50% - 50px);
}
</style>
