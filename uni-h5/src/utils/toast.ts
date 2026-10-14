/** 提示类交互的统一出口，避免各页面各写一套 toast 配置。 */

export function toast(title: string, icon: 'none' | 'success' | 'error' = 'none'): void {
  uni.showToast({ title, icon, duration: 2000 })
}

export function toastError(title: string): void {
  uni.showToast({ title, icon: 'none', duration: 2500 })
}

export function showLoading(title = '加载中'): void {
  uni.showLoading({ title, mask: true })
}

export function hideLoading(): void {
  uni.hideLoading()
}
