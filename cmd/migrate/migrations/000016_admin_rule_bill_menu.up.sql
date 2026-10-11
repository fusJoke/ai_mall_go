-- +migrate Up
--
-- admin_rule 菜单规则种子：批次账单管理（add-batch-settlement 任务 5.4）。
--   - 批次账单 → /src/views/admin/bills/index.vue
--
-- 背景：商品即批次 v2 —— 「1 商品 = 1 账单」，商品售罄时由抽卡事务自动生成
-- （internal/service/user/draw.go 事务尾部 → mall.BillService.GenerateForBox）。
-- admin 的职责是核对供应商驳回的账单（Regenerate 重核金额）与对已确认账单打款
-- （MarkPaid：bill→paid + supplier.balance -= payout + 商品置 settled）。
--
-- 静态子路由在 web/src/router/static/adminBase.ts（name: adminBill，
-- title: pageTitles.adminBill），此处补 admin_rule 菜单种子，两条链路独立可刷新。
--
-- admin_rule.name 无唯一索引（见 000002），ON DUPLICATE KEY 不生效；
-- 用 INSERT ... SELECT ... WHERE NOT EXISTS 实现真正可重入。
-- pid=0 顶级菜单；status=1 启用；weigh 与既有 mall 外菜单一致为 0。
-- 排序上放在结算管理之后（同 weigh，按 id 追加显示）。

INSERT INTO `admin_rule`
  (`pid`, `type`,  `title`,     `name`,        `path`,   `component`,                        `open_type`, `url`, `keepalive`, `extend`, `remark`, `weigh`, `status`, `updated_at`,           `created_at`)
SELECT 0,        'menu', '批次账单', 'admin/bills', 'bills', '/src/views/admin/bills/index.vue', NULL,        '',    0,           'none',   '',       0,       1,        CURRENT_TIMESTAMP(3), CURRENT_TIMESTAMP(3)
WHERE NOT EXISTS (SELECT 1 FROM `admin_rule` WHERE `name` = 'admin/bills' AND `deleted_at` IS NULL);
