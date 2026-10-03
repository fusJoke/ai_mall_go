// web\src\mock\menus.ts
// 后台示例菜单数据 —— init 响应 menus 为空时的前端兜底（openspec add-admin-layout-shell）。
//
// 数据形状与后端 InitService.routeFromRule 的输出严格一致：
//   { path, name, component, meta: { title, icon, menuType, weigh, id, pid } }
// menuType: 'dir' = 目录（不注册路由，仅用于菜单层级展示）；'menu' = 可导航叶子；
//           'iframe' = 内嵌页面（path 为外链 URL，注册时挂 iframe 视图）。
//
// 叶子全部指向真实存在的视图（/src/views/...），目录仅演示 el-sub-menu 层级；
// component 为字符串由 layouts/admin 的 registerMenuRoutes 经 import.meta.glob 解析。

export interface MockMenuItem {
    path: string
    name: string
    component?: string
    meta: {
        title: string
        icon: string
        menuType: 'dir' | 'menu' | 'iframe'
        weigh: number
        id: number
        pid: number
    }
    children?: MockMenuItem[]
}

export const exampleAdminMenus: MockMenuItem[] = [
    {
        path: 'dashboard',
        name: 'admin/dashboard',
        component: '/src/views/admin/dashboard.vue',
        meta: { title: '后台主页', icon: 'lucide:Home', menuType: 'menu', weigh: 0, id: 101, pid: 0 },
    },
    {
        path: 'system',
        name: 'admin/system',
        meta: { title: '权限管理', icon: 'lucide:Settings', menuType: 'dir', weigh: 1, id: 102, pid: 0 },
        children: [
            {
                path: 'manager',
                name: 'admin/manager',
                component: '/src/views/admin/manager/index.vue',
                meta: { title: '管理员账号', icon: 'lucide:Users', menuType: 'menu', weigh: 0, id: 103, pid: 102 },
            },
            {
                path: 'rule',
                name: 'admin/rule',
                component: '/src/views/admin/rule/index.vue',
                meta: { title: '菜单规则', icon: 'lucide:ListTree', menuType: 'menu', weigh: 1, id: 104, pid: 102 },
            },
            {
                path: 'group',
                name: 'admin/group',
                component: '/src/views/admin/group/index.vue',
                meta: { title: '角色分组', icon: 'lucide:ShieldCheck', menuType: 'menu', weigh: 2, id: 106, pid: 102 },
            },
            {
                path: 'log',
                name: 'admin/log',
                component: '/src/views/admin/log/index.vue',
                meta: { title: '管理员日志', icon: 'lucide:ScrollText', menuType: 'menu', weigh: 3, id: 107, pid: 102 },
            },
        ],
    },
    {
        path: 'routine',
        name: 'admin/routine',
        meta: { title: '常规管理', icon: 'lucide:FolderCog', menuType: 'dir', weigh: 2, id: 108, pid: 0 },
        children: [
            {
                path: 'agInput',
                name: 'admin/agInput',
                component: '/src/views/agInput/index.vue',
                meta: { title: '多驱动上传演示', icon: 'lucide:UploadCloud', menuType: 'menu', weigh: 0, id: 109, pid: 108 },
            },
            {
                path: 'https://demo.buildadmin.com',
                name: 'admin/iframe-demo',
                meta: { title: 'iframe 示例', icon: 'lucide:Globe', menuType: 'iframe', weigh: 1, id: 110, pid: 108 },
            },
        ],
    },
]
