<script setup lang="ts">
/**
 * P8 我的。
 *
 * 余额走 `GET /user/profile`（而不是读登录时的快照），保证下单后能看到最新值。
 * 按已确认决策：本页**不含**联系客服入口（tasks 9.3）。
 */

import { computed, ref } from 'vue'
import { onShow } from '@dcloudio/uni-app'

import NavBar from '@/components/nav-bar.vue'
import { logout as apiLogout } from '@/api/auth'
import { fetchProfile } from '@/api/profile'
import { authState, clearAuth, isLoggedIn, patchUser } from '@/store/user'
import { formatPrice } from '@/utils/format'
import { toast, toastError } from '@/utils/toast'
import type { ApiError } from '@/types'

const refreshing = ref(false)

const user = computed(() => authState.user)
const loggedIn = computed(() => isLoggedIn())
const balance = computed(() => formatPrice(user.value?.balance))

async function refreshBalance(): Promise<void> {
  if (refreshing.value || !loggedIn.value) return
  refreshing.value = true
  try {
    const profile = await fetchProfile()
    patchUser(profile)
    toast('余额已更新')
  } catch (err) {
    toastError((err as ApiError).message || '网络异常，请稍后重试')
  } finally {
    refreshing.value = false
  }
}

function goLogin(): void {
  uni.navigateTo({ url: '/pages/login/index?redirect=%2Fpages%2Fmine%2Findex' })
}

function goOrders(): void {
  if (!loggedIn.value) {
    goLogin()
    return
  }
  uni.switchTab({ url: '/pages/orders/index' })
}

function onLogout(): void {
  uni.showModal({
    title: '退出登录',
    content: '确认退出当前账号？',
    success: async (res) => {
      if (!res.confirm) return
      try {
        await apiLogout()
      } catch (err) {
        // 登出接口幂等，网络失败也不阻断本地清理。
        console.warn('[mine] logout request failed:', err)
      }
      clearAuth()
      uni.switchTab({ url: '/pages/home/index' })
    },
  })
}

onShow(() => {
  if (!loggedIn.value) return
  void refreshBalance()
})
</script>

<template>
  <view class="page mine">
    <NavBar title="我的" />

    <!-- 用户卡 -->
    <view class="profile">
      <view class="profile__avatar">
        <text class="profile__initial">{{ (user?.nickname || '玩')[0] }}</text>
      </view>
      <view class="profile__info">
        <text class="profile__name ellipsis">
          {{ user?.nickname || '未登录' }}
        </text>
        <text class="profile__mobile">{{ user?.mobile || user?.username || '-' }}</text>
      </view>
      <view v-if="!loggedIn" class="profile__login" @tap="goLogin">
        <text>去登录</text>
      </view>
    </view>

    <!-- 余额卡 -->
    <view class="balance">
      <view class="row--between">
        <text class="balance__label">账户余额</text>
        <text class="balance__refresh" @tap="refreshBalance">
          {{ refreshing ? '加载中' : '刷新余额' }}
        </text>
      </view>
      <text class="balance__value">
        <text class="balance__symbol">¥</text>{{ balance }}
      </text>
      <text v-if="authState.degraded" class="balance__warn">当前是本地 mock 会话，下单等操作不可用</text>
    </view>

    <!-- 菜单 -->
    <view class="card menu">
      <view class="menu__item" @tap="goOrders">
        <text class="menu__text">我的订单</text>
        <text class="menu__arrow">›</text>
      </view>
      <view class="menu__item" @tap="refreshBalance">
        <text class="menu__text">刷新余额</text>
        <text class="menu__arrow">›</text>
      </view>
    </view>

    <view v-if="loggedIn" class="btn btn--ghost logout" @tap="onLogout">
      <text>退出登录</text>
    </view>
  </view>
</template>

<style lang="scss" scoped>
.mine {
  padding: 0 var(--space-md);
}

.profile {
  display: flex;
  align-items: center;
  padding: var(--space-md) 0 var(--space-lg);
}

.profile__avatar {
  width: 112rpx;
  height: 112rpx;
  border-radius: 50%;
  background: linear-gradient(135deg, var(--surface-2), var(--surface));
  border: 1rpx solid var(--border);
  display: flex;
  align-items: center;
  justify-content: center;
}

.profile__initial {
  font-size: 44rpx;
  font-weight: 700;
  color: var(--gold);
}

.profile__info {
  flex: 1;
  min-width: 0;
  margin-left: var(--space-md);
}

.profile__name {
  display: block;
  font-size: 34rpx;
  font-weight: 600;
  color: var(--text);
}

.profile__mobile {
  display: block;
  margin-top: 4rpx;
  font-size: 24rpx;
  color: var(--dim);
}

.profile__login {
  padding: 12rpx 28rpx;
  border-radius: var(--radius-pill);
  border: 1rpx solid var(--gold);
  color: var(--gold);
  font-size: 26rpx;
}

.balance {
  padding: var(--space-lg) var(--space-md);
  border-radius: var(--radius-lg);
  background: linear-gradient(135deg, rgba(245, 181, 68, 0.16), rgba(185, 140, 255, 0.1));
  border: 1rpx solid var(--border);
}

.balance__label {
  font-size: 24rpx;
  color: var(--muted);
}

.balance__refresh {
  font-size: 24rpx;
  color: var(--gold);
}

.balance__value {
  display: block;
  margin-top: var(--space-sm);
  font-size: 56rpx;
  font-weight: 700;
  color: var(--gold-2);
}

.balance__symbol {
  font-size: 30rpx;
  margin-right: 4rpx;
}

.balance__warn {
  display: block;
  margin-top: var(--space-sm);
  font-size: 22rpx;
  color: var(--danger);
}

.menu {
  margin-top: var(--space-md);
  padding: 0 var(--space-md);
}

.menu__item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 100rpx;
}

.menu__item + .menu__item {
  border-top: 1rpx solid var(--border);
}

.menu__text {
  font-size: 28rpx;
  color: var(--text);
}

.menu__arrow {
  font-size: 34rpx;
  color: var(--dim);
}

.logout {
  margin-top: var(--space-lg);
}
</style>
