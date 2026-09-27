<template>
    <div class="home">
        <!-- 顶部导航 -->
        <header class="topbar" :class="{ scrolled: scrolled }">
            <div class="topbar-inner">
                <a class="brand" href="#top" @click.prevent="scrollTo('#top')">
                    <el-icon class="brand-icon"><Goods /></el-icon>
                    <span class="brand-name">AI GO MALL</span>
                    <span class="brand-tag">v1.0</span>
                </a>
                <nav class="nav">
                    <a href="#tech" class="nav-link" @click.prevent="scrollTo('#tech')">
                        {{ t('techTitle') }}
                    </a>
                    <a href="#philosophy" class="nav-link" @click.prevent="scrollTo('#philosophy')">
                        {{ t('philosophyTitle') }}
                    </a>
                    <a href="#blog" class="nav-link" @click.prevent="scrollTo('#blog')">
                        {{ t('blogTitle') }}
                    </a>
                </nav>
                <a class="nav-cta" href="#/admin/login">
                    {{ t('enterAdmin') }}
                    <el-icon class="el-icon--right"><ArrowRight /></el-icon>
                </a>
            </div>
        </header>

        <!-- Hero -->
        <section id="top" class="hero">
            <div class="hero-inner">
                <div class="hero-badge">
                    <el-icon><MagicStick /></el-icon>
                    <span>{{ t('heroBadge') }}</span>
                </div>
                <h1 class="hero-title">
                    {{ t('heroTitle') }}
                </h1>
                <p class="hero-subtitle">{{ t('heroSubtitle') }}</p>
                <p class="hero-desc">{{ t('heroDesc') }}</p>
                <div class="hero-cta">
                    <a class="btn-primary" href="#/admin/login">
                        {{ t('enterAdmin') }}
                        <el-icon class="el-icon--right"><ArrowRight /></el-icon>
                    </a>
                    <a class="btn-ghost" href="#tech" @click.prevent="scrollTo('#tech')">
                        {{ t('learnTech') }}
                    </a>
                </div>

                <div class="hero-scroll">
                    <el-icon><ArrowDown /></el-icon>
                    <span>{{ t('scrollDown') }}</span>
                </div>
            </div>
            <div class="hero-decor" aria-hidden="true">
                <div class="hero-decor-blur hero-decor-blur-1" />
                <div class="hero-decor-blur hero-decor-blur-2" />
                <div class="hero-decor-grid" />
            </div>
        </section>

        <!-- 技术栈 -->
        <section id="tech" class="section">
            <div class="section-inner">
                <div class="section-head">
                    <span class="section-eyebrow">{{ t('techTitle') }}</span>
                    <h2 class="section-title">{{ t('techSubtitle') }}</h2>
                </div>
                <div class="tech-grid">
                    <article
                        v-for="(item, idx) in techStack"
                        :key="item.title"
                        class="tech-card"
                        :style="{ animationDelay: `${idx * 60}ms` }"
                    >
                        <div class="tech-icon" :style="{ background: item.bg, color: item.fg }">
                            <el-icon :size="24">
                                <component :is="item.icon" />
                            </el-icon>
                        </div>
                        <h3 class="tech-name">{{ item.title }}</h3>
                        <p class="tech-desc">{{ item.desc }}</p>
                    </article>
                </div>
            </div>
        </section>

        <!-- 开发理念 -->
        <section id="philosophy" class="section section-alt">
            <div class="section-inner">
                <div class="section-head">
                    <span class="section-eyebrow">{{ t('philosophyTitle') }}</span>
                    <h2 class="section-title">{{ t('philosophySubtitle') }}</h2>
                </div>
                <div class="philosophy-grid">
                    <article class="ph-card ph-card-human">
                        <div class="ph-icon">
                            <el-icon :size="28"><User /></el-icon>
                        </div>
                        <h3 class="ph-title">{{ t('humanCommand') }}</h3>
                        <p class="ph-desc">{{ t('humanDesc') }}</p>
                        <div class="ph-tag">舵手 · 决策</div>
                    </article>
                    <article class="ph-card ph-card-ai">
                        <div class="ph-icon">
                            <el-icon :size="28"><ChatLineRound /></el-icon>
                        </div>
                        <h3 class="ph-title">{{ t('aiWriter') }}</h3>
                        <p class="ph-desc">{{ t('aiDesc') }}</p>
                        <div class="ph-tag">执笔 · 提速</div>
                    </article>
                </div>
            </div>
        </section>

        <!-- 开源博客 -->
        <section id="blog" class="section">
            <div class="section-inner section-inner-narrow">
                <div class="section-head">
                    <span class="section-eyebrow">{{ t('blogTitle') }}</span>
                    <h2 class="section-title">{{ t('blogSubtitle') }}</h2>
                </div>
                <div class="blog-placeholder">
                    <el-icon :size="40" class="blog-icon"><Document /></el-icon>
                    <p>持续记录中，欢迎共建。</p>
                    <span class="blog-hint">即将上线</span>
                </div>
            </div>
        </section>

        <!-- 底部 -->
        <footer class="footer">
            <div class="footer-inner">
                <div class="footer-brand">
                    <el-icon class="brand-icon"><Goods /></el-icon>
                    <span>{{ t('footerTagline') }}</span>
                </div>
                <div class="footer-meta">
                    <span>© 2026 AI GO MALL</span>
                    <span class="dot">·</span>
                    <span>Module: ai-go-mall</span>
                </div>
            </div>
        </footer>
    </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import {
    Goods,
    ArrowRight,
    ArrowDown,
    MagicStick,
    Box,
    Document,
    Coin,
    Connection,
    Brush,
    User,
    ChatLineRound,
} from '@element-plus/icons-vue'

const { t } = useI18n()
const scrolled = ref(false)

function onScroll() {
    scrolled.value = window.scrollY > 8
}

function scrollTo(sel: string) {
    const el = document.querySelector(sel) as HTMLElement | null
    if (el) el.scrollIntoView({ behavior: 'smooth', block: 'start' })
}

onMounted(() => {
    window.addEventListener('scroll', onScroll, { passive: true })
    onScroll()
})
onUnmounted(() => {
    window.removeEventListener('scroll', onScroll)
})

interface TechItem {
    icon: any
    title: string
    desc: string
    bg: string
    fg: string
}

const techStack: TechItem[] = [
    {
        icon: Box,
        title: 'Golang',
        desc: t('techGolang'),
        bg: 'rgba(79, 70, 229, 0.08)',
        fg: '#4f46e5',
    },
    {
        icon: Document,
        title: 'TypeScript',
        desc: t('techTypeScript'),
        bg: 'rgba(14, 165, 233, 0.08)',
        fg: '#0284c7',
    },
    {
        icon: Coin,
        title: 'PostgreSQL',
        desc: t('techPostgreSQL'),
        bg: 'rgba(99, 102, 241, 0.08)',
        fg: '#6366f1',
    },
    {
        icon: Connection,
        title: 'Vue 3',
        desc: t('techVue'),
        bg: 'rgba(16, 185, 129, 0.08)',
        fg: '#059669',
    },
    {
        icon: Brush,
        title: 'Element Plus',
        desc: t('techElementPlus'),
        bg: 'rgba(245, 158, 11, 0.08)',
        fg: '#d97706',
    },
    {
        icon: ChatLineRound,
        title: 'Claude Code',
        desc: t('techClaudeCode'),
        bg: 'rgba(236, 72, 153, 0.08)',
        fg: '#db2777',
    },
]
</script>

<style scoped lang="scss">
// ─────── Tokens ───────
$primary: #4f46e5;
$primary-dark: #4338ca;
$primary-light: #818cf8;
$text-1: #0f172a;
$text-2: #475569;
$text-3: #94a3b8;
$border: #e2e8f0;
$bg-soft: #fafafb;

// ─────── Reset / base ───────
.home {
    background: #fff;
    color: $text-1;
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC',
        'Hiragino Sans GB', 'Microsoft YaHei', sans-serif;
    -webkit-font-smoothing: antialiased;
    -moz-osx-font-smoothing: grayscale;
    line-height: 1.6;
}
.home a {
    color: inherit;
    text-decoration: none;
}

// ─────── 顶部导航 ───────
.topbar {
    position: sticky;
    top: 0;
    z-index: 50;
    background: rgba(255, 255, 255, 0.6);
    backdrop-filter: saturate(180%) blur(12px);
    -webkit-backdrop-filter: saturate(180%) blur(12px);
    border-bottom: 1px solid transparent;
    transition: background 0.2s ease, border-color 0.2s ease;

    &.scrolled {
        background: rgba(255, 255, 255, 0.85);
        border-bottom-color: $border;
    }
}
.topbar-inner {
    max-width: 1200px;
    margin: 0 auto;
    height: 64px;
    padding: 0 32px;
    display: flex;
    align-items: center;
    gap: 24px;
}
.brand {
    display: flex;
    align-items: center;
    gap: 8px;
    font-weight: 600;
    font-size: 16px;
    color: $text-1;
    cursor: pointer;

    .brand-icon {
        color: $primary;
        font-size: 22px;
    }
    .brand-tag {
        font-size: 11px;
        padding: 2px 6px;
        background: rgba(79, 70, 229, 0.1);
        color: $primary;
        border-radius: 4px;
        font-weight: 500;
        letter-spacing: 0.04em;
    }
}
.nav {
    display: flex;
    align-items: center;
    gap: 28px;
    margin-left: auto;

    .nav-link {
        color: $text-2;
        font-size: 14px;
        font-weight: 500;
        transition: color 0.15s;

        &:hover {
            color: $text-1;
        }
    }
}
.nav-cta {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    height: 36px;
    padding: 0 16px;
    background: $primary;
    color: #fff;
    border-radius: 8px;
    font-size: 14px;
    font-weight: 500;
    transition: background 0.15s, transform 0.15s;

    &:hover {
        background: $primary-dark;
    }
    &:active {
        transform: translateY(1px);
    }
}

@media (max-width: 768px) {
    .nav { display: none; }
    .topbar-inner { padding: 0 16px; }
}

// ─────── Hero ───────
.hero {
    position: relative;
    overflow: hidden;
    padding: 96px 24px 120px;
    text-align: center;
    background: radial-gradient(
            ellipse at 50% 0%,
            rgba(79, 70, 229, 0.08),
            transparent 65%
        ),
        #fff;
}
.hero-inner {
    position: relative;
    max-width: 880px;
    margin: 0 auto;
    z-index: 1;
}
.hero-badge {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 6px 14px;
    margin-bottom: 32px;
    background: rgba(79, 70, 229, 0.08);
    color: $primary;
    border: 1px solid rgba(79, 70, 229, 0.15);
    border-radius: 999px;
    font-size: 13px;
    font-weight: 500;
}
.hero-title {
    font-size: clamp(44px, 8vw, 80px);
    line-height: 1.05;
    font-weight: 700;
    letter-spacing: -0.04em;
    margin: 0 0 24px;
    background: linear-gradient(180deg, $text-1 0%, #334155 100%);
    -webkit-background-clip: text;
    background-clip: text;
    color: transparent;
}
.hero-subtitle {
    margin: 0 0 12px;
    font-size: 20px;
    font-weight: 500;
    color: $primary;
    letter-spacing: -0.01em;
}
.hero-desc {
    max-width: 620px;
    margin: 0 auto 40px;
    font-size: 16px;
    line-height: 1.7;
    color: $text-2;
}
.hero-cta {
    display: flex;
    justify-content: center;
    gap: 12px;
    flex-wrap: wrap;
}
.btn-primary {
    display: inline-flex;
    align-items: center;
    height: 48px;
    padding: 0 24px;
    background: $primary;
    color: #fff;
    border-radius: 10px;
    font-size: 15px;
    font-weight: 500;
    box-shadow: 0 4px 14px -4px rgba(79, 70, 229, 0.5);
    transition: background 0.15s, transform 0.15s, box-shadow 0.15s;

    &:hover {
        background: $primary-dark;
        box-shadow: 0 8px 22px -4px rgba(79, 70, 229, 0.55);
    }
    &:active {
        transform: translateY(1px);
    }
}
.btn-ghost {
    display: inline-flex;
    align-items: center;
    height: 48px;
    padding: 0 22px;
    background: #fff;
    color: $text-1;
    border: 1px solid $border;
    border-radius: 10px;
    font-size: 15px;
    font-weight: 500;
    transition: border-color 0.15s, color 0.15s;

    &:hover {
        border-color: $primary;
        color: $primary;
    }
}
.hero-scroll {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    margin-top: 72px;
    color: $text-3;
    font-size: 12px;
    letter-spacing: 0.06em;
    text-transform: uppercase;
}

// Hero 装饰
.hero-decor {
    position: absolute;
    inset: 0;
    pointer-events: none;
    overflow: hidden;
    z-index: 0;
}
.hero-decor-blur {
    position: absolute;
    border-radius: 50%;
    filter: blur(100px);
    opacity: 0.7;
}
.hero-decor-blur-1 {
    top: -120px;
    left: -120px;
    width: 420px;
    height: 420px;
    background: rgba(79, 70, 229, 0.16);
}
.hero-decor-blur-2 {
    bottom: -180px;
    right: -120px;
    width: 360px;
    height: 360px;
    background: rgba(129, 140, 248, 0.18);
}
.hero-decor-grid {
    position: absolute;
    inset: 0;
    background-image:
        linear-gradient(to right, rgba(15, 23, 42, 0.04) 1px, transparent 1px),
        linear-gradient(to bottom, rgba(15, 23, 42, 0.04) 1px, transparent 1px);
    background-size: 56px 56px;
    mask-image: radial-gradient(ellipse at center, black, transparent 70%);
    -webkit-mask-image: radial-gradient(ellipse at center, black, transparent 70%);
}

// ─────── 通用 section ───────
.section {
    padding: 112px 24px;
}
.section-alt {
    background: $bg-soft;
}
.section-inner {
    max-width: 1200px;
    margin: 0 auto;
}
.section-inner-narrow {
    max-width: 760px;
}
.section-head {
    text-align: center;
    margin-bottom: 64px;
}
.section-eyebrow {
    display: inline-block;
    padding: 4px 12px;
    background: rgba(79, 70, 229, 0.08);
    color: $primary;
    border-radius: 999px;
    font-size: 12px;
    font-weight: 600;
    letter-spacing: 0.1em;
    text-transform: uppercase;
    margin-bottom: 18px;
}
.section-title {
    font-size: clamp(28px, 4vw, 40px);
    line-height: 1.25;
    font-weight: 700;
    letter-spacing: -0.02em;
    color: $text-1;
    margin: 0;
}

// ─────── Tech 卡片 ───────
.tech-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(260px, 1fr));
    gap: 20px;
}
.tech-card {
    padding: 28px;
    background: #fff;
    border: 1px solid $border;
    border-radius: 14px;
    transition: transform 0.2s ease, box-shadow 0.2s ease, border-color 0.2s ease;

    &:hover {
        transform: translateY(-3px);
        box-shadow: 0 16px 32px -16px rgba(15, 23, 42, 0.15);
        border-color: rgba(79, 70, 229, 0.25);
    }
}
.tech-icon {
    width: 48px;
    height: 48px;
    border-radius: 12px;
    display: flex;
    align-items: center;
    justify-content: center;
    margin-bottom: 20px;
}
.tech-name {
    font-size: 18px;
    font-weight: 600;
    color: $text-1;
    margin: 0 0 8px;
    letter-spacing: -0.01em;
}
.tech-desc {
    font-size: 14px;
    line-height: 1.7;
    color: $text-2;
    margin: 0;
}

// ─────── Philosophy 卡片 ───────
.philosophy-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(320px, 1fr));
    gap: 24px;
}
.ph-card {
    position: relative;
    padding: 36px;
    background: #fff;
    border: 1px solid $border;
    border-radius: 16px;
    overflow: hidden;
    transition: transform 0.2s ease, box-shadow 0.2s ease;

    &:hover {
        transform: translateY(-3px);
        box-shadow: 0 20px 40px -16px rgba(15, 23, 42, 0.15);
    }
}
.ph-card-human {
    background: linear-gradient(135deg, #4f46e5 0%, #6366f1 100%);
    color: #fff;
    border-color: transparent;
}
.ph-card-ai {
    background: linear-gradient(135deg, #1e293b 0%, #0f172a 100%);
    color: #fff;
    border-color: transparent;
}
.ph-icon {
    width: 56px;
    height: 56px;
    border-radius: 14px;
    background: rgba(255, 255, 255, 0.15);
    backdrop-filter: blur(8px);
    -webkit-backdrop-filter: blur(8px);
    display: flex;
    align-items: center;
    justify-content: center;
    margin-bottom: 24px;
}
.ph-title {
    font-size: 22px;
    font-weight: 600;
    margin: 0 0 12px;
    letter-spacing: -0.01em;
}
.ph-desc {
    font-size: 15px;
    line-height: 1.75;
    opacity: 0.85;
    margin: 0 0 24px;
}
.ph-tag {
    display: inline-block;
    padding: 4px 12px;
    background: rgba(255, 255, 255, 0.15);
    border-radius: 999px;
    font-size: 12px;
    font-weight: 500;
    letter-spacing: 0.05em;
    backdrop-filter: blur(8px);
    -webkit-backdrop-filter: blur(8px);
}

// ─────── Blog 占位 ───────
.blog-placeholder {
    padding: 56px 24px;
    text-align: center;
    border: 1px dashed $border;
    border-radius: 16px;
    background: #fff;

    .blog-icon {
        color: $primary;
        margin-bottom: 16px;
    }
    p {
        margin: 0 0 8px;
        color: $text-1;
        font-size: 16px;
    }
    .blog-hint {
        display: inline-block;
        padding: 2px 10px;
        background: rgba(79, 70, 229, 0.08);
        color: $primary;
        border-radius: 999px;
        font-size: 12px;
        font-weight: 500;
    }
}

// ─────── Footer ───────
.footer {
    border-top: 1px solid $border;
    padding: 32px 24px;
    background: #fff;
}
.footer-inner {
    max-width: 1200px;
    margin: 0 auto;
    display: flex;
    align-items: center;
    justify-content: space-between;
    flex-wrap: wrap;
    gap: 16px;
}
.footer-brand {
    display: flex;
    align-items: center;
    gap: 8px;
    font-weight: 600;
    color: $text-1;

    .brand-icon {
        color: $primary;
        font-size: 20px;
    }
}
.footer-meta {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 13px;
    color: $text-3;
    flex-wrap: wrap;
    .dot { color: $text-3; }
}

// ─────── 响应式 ───────
@media (max-width: 640px) {
    .hero { padding: 72px 16px 88px; }
    .section { padding: 72px 16px; }
    .section-head { margin-bottom: 40px; }
    .ph-card { padding: 28px; }
    .footer-inner { flex-direction: column; text-align: center; }
}
</style>
