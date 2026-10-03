// web\src\mock\adminLogs.ts
// 管理员日志示例数据 —— 服务端 admin_log 表未实现前的前端兜底（示例数据约定）。
// 字段对齐 buildadmin 的 admin_log 表：id / admin_id / username / title / url / ip / useragent / createtime。

export interface AdminLog {
    id: number
    admin_id: number
    username: string
    title: string
    url: string
    ip: string
    useragent: string
    createtime: string
}

/** 操作样本：title + 对应请求路径 */
const actions: { title: string; url: string }[] = [
    { title: '登录后台', url: '/admin/login' },
    { title: '后台初始化', url: '/admin/init' },
    { title: '查看管理员列表', url: '/admin/admin/list' },
    { title: '新增管理员', url: '/admin/admin/create' },
    { title: '编辑管理员', url: '/admin/admin/edit' },
    { title: '修改密码', url: '/admin/admin/change-password' },
    { title: '查看菜单规则', url: '/admin/rule/list' },
    { title: '新增菜单规则', url: '/admin/rule/create' },
    { title: '编辑菜单规则', url: '/admin/rule/edit' },
    { title: '查看角色分组', url: '/admin/group/list' },
    { title: '保存角色权限', url: '/admin/group/edit' },
    { title: '上传附件', url: '/admin/ajax/upload' },
    { title: '退出登录', url: '/admin/logout' },
]

const admins = [
    { id: 1, username: 'admin' },
    { id: 2, username: 'editor' },
    { id: 3, username: 'auditor' },
]

const ips = ['192.168.1.10', '192.168.1.23', '10.0.0.8', '172.16.0.15', '127.0.0.1']

const useragents = [
    'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36',
    'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Safari/605.1.15',
    'Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:127.0) Gecko/20100101 Firefox/127.0',
]

/**
 * 确定性生成 66 条示例日志（固定种子，无随机抖动，便于分页/搜索测试可复现）。
 * createtime 从 2026-09-28 起按 3 小时步进倒推。
 */
function buildExampleAdminLogs(): AdminLog[] {
    const logs: AdminLog[] = []
    const start = new Date('2026-09-28T09:00:00+08:00').getTime()
    for (let i = 0; i < 66; i++) {
        const action = actions[i % actions.length]
        const admin = admins[i % admins.length]
        const time = new Date(start - i * 3 * 3600 * 1000)
        const pad = (n: number) => String(n).padStart(2, '0')
        logs.push({
            id: 1000 + i,
            admin_id: admin.id,
            username: admin.username,
            title: action.title,
            url: action.url,
            ip: ips[i % ips.length],
            useragent: useragents[i % useragents.length],
            createtime: `${time.getFullYear()}-${pad(time.getMonth() + 1)}-${pad(time.getDate())} ${pad(time.getHours())}:${pad(time.getMinutes())}:${pad(time.getSeconds())}`,
        })
    }
    return logs
}

export const exampleAdminLogs: AdminLog[] = buildExampleAdminLogs()
