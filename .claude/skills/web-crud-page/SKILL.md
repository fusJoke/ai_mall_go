---
name: web-crud-page
description: 在 web/src/views/ 下新增或改造页面时使用，尤其是 Element Plus 的列表页、表单页、弹窗。规定页面骨架、搜索分页、三态处理、类型化 props/emits 与目录命名。写任何 .vue 页面时加载。
---

# 页面开发规范（Element Plus 三端）

## 何时用

新增或重构 `web/src/views/{user,supplier,admin}/**` 下的页面；给列表加搜索/分页；写表单弹窗。

## 目录与命名

- 路径与 api 身份镜像：`views/user/`、`views/supplier/`、`views/admin/`
- **页面**文件小写（`productList.vue` 或 `products/index.vue`），**组件** PascalCase（`CardTable.vue`）
- 路由不写在页面里：静态路由加在 `src/router/static/{userBase,supplierBase,adminBase}.ts`
- 布局用 `src/layouts/` 现有壳，禁止在页面里重复实现侧边栏/顶栏

## 列表页骨架（固定六件套）

1. **搜索区**：`el-form inline` + 查询/重置按钮，条件收在 `reactive` 对象里
2. **操作区**：新增/批量操作按钮，按权限显隐
3. **表格**：`el-table` + `v-loading`，每列显式 `prop`/`label`，操作列 `fixed="right"`
4. **分页**：`el-pagination`，`page`/`page_size` 与后端字段对齐，切页保留搜索条件
5. **三态**：loading / empty（`el-empty`）/ error（重试按钮）——**缺一不可**
6. **弹窗**：新增/编辑用 `el-dialog` + `el-form`，`ref` 拿 `validate()`，提交前 `await formRef.validate()`

## 代码骨架（照抄结构）

```vue
<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { ElMessage, type FormInstance } from 'element-plus'
import { orderList, type DrawOrder } from '/@/api/user/order'

const loading = ref(false)
const list = ref<DrawOrder[]>([])
const total = ref(0)
const query = reactive({ page: 1, page_size: 20, keyword: '' })

async function fetchList() {
    loading.value = true
    try {
        const res = await orderList(query)
        list.value = res.items
        total.value = res.total
    } finally {
        loading.value = false // 异常路径也必须复位
    }
}

onMounted(fetchList)
</script>
```

## 硬规则

1. `<script setup lang="ts">` + Composition API。**禁止** Options API、mixins、`this`。
2. props/emits 必须类型化：`defineProps<{...}>()` / `defineEmits<{...}>()`；双向绑定用 `defineModel<T>()`，**不要**手写 `modelValue` + `update:modelValue`。
3. **禁止 `any`**。第三方库类型缺失时用 `unknown` + 类型收窄，或在 `web/types/` 补声明。
4. 请求必须 `try/finally` 关 loading——异常路径不复位会导致页面永久转圈。
5. 数据获取统一走 `src/api/` 的函数，**禁止在 .vue 里手写 url 或直接调 axios**。
6. 图标用 `@element-plus/icons-vue` 或 `@lucide/vue`（项目已装两者），不要引入新图标库。
7. 用户可见文案沿用 `src/lang/` 的 i18n 方式（新增文案前先 grep 现有用法对齐 key 结构，不要自创规范）。
8. 样式用 `<style scoped lang="scss">`，颜色/间距从 `src/styles/` 取变量，不写魔法色值。
9. 大表格/长列表必须分页或虚拟滚动，禁止一次渲染上千行。

## 收尾自检

- [ ] `pnpm -C web typecheck` 通过
- [ ] 三态齐全（loading / empty / error）
- [ ] 无 `any`、无 Options API、无手写 URL
- [ ] `pnpm -C web lint` 通过
