<template>
    <div class="loading">
        <!--
            吃豆人加载动画：纯 HTML/CSS 绘制（spec admin-init「loading 页吃豆人动画」）。
            上/下颚两片半圆以 transform-origin 在嘴角反向旋转形成"张嘴-闭合"循环；
            三颗豆子错峰向嘴部平移并在到达嘴边时淡出，视觉上被"吃掉"。
            prefers-reduced-motion: reduce 时停用动画（见样式底部 media query）。
        -->
        <div class="pacman-scene" role="status" aria-label="加载中">
            <div class="dots">
                <span class="dot dot-1" />
                <span class="dot dot-2" />
                <span class="dot dot-3" />
            </div>
            <div class="pacman">
                <span class="jaw jaw-top" />
                <span class="jaw jaw-bottom" />
            </div>
        </div>
        <span class="loading-text">加载中...</span>
    </div>
</template>

<script setup lang="ts"></script>

<style scoped lang="scss">
.loading {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 20px;
    width: 100%;
    height: 100%;
    min-height: 200px;
}
.loading-text {
    color: var(--el-text-color-secondary);
    font-size: var(--el-font-size-base);
}

/* 场景：吃豆人在左（嘴缘约 x=40px），豆子从右被吃入；豆子各自终止在嘴边 */
.pacman-scene {
    position: relative;
    display: flex;
    align-items: center;
    width: 160px;
    height: 48px;
}

/* 吃豆人主体：两片半圆合成圆形，旋转处即嘴 */
.pacman {
    position: absolute;
    left: 0;
    width: 40px;
    height: 40px;
    z-index: 1;

    .jaw {
        position: absolute;
        left: 0;
        width: 40px;
        height: 20px;
        background: #f7c948;
    }
    .jaw-top {
        top: 0;
        border-radius: 40px 40px 0 0;
        transform-origin: 50% 100%;
        animation: jaw-top-chomp 0.32s ease-in-out infinite;
    }
    .jaw-bottom {
        top: 20px;
        border-radius: 0 0 40px 40px;
        transform-origin: 50% 0;
        animation: jaw-bottom-chomp 0.32s ease-in-out infinite;
    }
}

@keyframes jaw-top-chomp {
    0%,
    100% {
        transform: rotate(0deg);
    }
    50% {
        transform: rotate(-34deg);
    }
}
@keyframes jaw-bottom-chomp {
    0%,
    100% {
        transform: rotate(0deg);
    }
    50% {
        transform: rotate(34deg);
    }
}

/* 豆子：同速从右向左（单 keyframe），行进 120px 终止在嘴缘（x≈40px）淡出；
   负 delay 使三颗豆子以固定 40px 间距形成连续进食流，不会从吃豆人左侧穿出 */
.dots {
    position: absolute;
    inset: 0;

    .dot {
        position: absolute;
        top: 50%;
        left: 160px;
        width: 8px;
        height: 8px;
        margin-top: -4px;
        border-radius: 50%;
        background: #f7c948;
        opacity: 0;
        animation: dot-feed 1.08s linear infinite;
    }
    .dot-1 {
        animation-delay: -1.08s;
    }
    .dot-2 {
        animation-delay: -0.72s;
    }
    .dot-3 {
        animation-delay: -0.36s;
    }
}

@keyframes dot-feed {
    0% {
        transform: translateX(0);
        opacity: 0;
    }
    15%,
    75% {
        opacity: 0.9;
    }
    100% {
        transform: translateX(-120px);
        opacity: 0;
    }
}

/* 减弱动态效果：停用所有循环动画，静态展示吃豆人与豆子 */
@media (prefers-reduced-motion: reduce) {
    .pacman .jaw,
    .dots .dot {
        animation: none;
    }
    .pacman .jaw-top {
        transform: rotate(-34deg);
    }
    .pacman .jaw-bottom {
        transform: rotate(34deg);
    }
    .dots .dot {
        opacity: 0.9;
    }
}
</style>
