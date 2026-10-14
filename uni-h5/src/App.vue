<script setup lang="ts">
import { onLaunch } from '@dcloudio/uni-app'

import { USE_MOCK, DEV_TOKEN } from '@/config'

onLaunch(() => {
  if (!USE_MOCK) return
  if (!DEV_TOKEN) {
    console.warn(
      '[uni-h5] 未配置 VITE_DEV_TOKEN：当前是 mock-only 会话，' +
        '抽卡 / 订单等需鉴权接口会返回 401。用法见 README「开发令牌」。'
    )
  }
})
</script>

<style lang="scss">
@import './styles/common.scss';
</style>
