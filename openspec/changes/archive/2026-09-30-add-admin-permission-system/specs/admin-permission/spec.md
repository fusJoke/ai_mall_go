# Spec Delta

## Purpose

后台管理员权限校验的数据底座与运行时 API。通过 admin_group（分组）、admin_group_access（管理员与分组的多对多关系）、admin_rule（菜单与权限节点）三张表，配合基于用户 id 的查询接口，让调用方能够判定「某管理员是否拥有某条规则」「某管理员的全部规则 / 分组 / 规则 ID」「某管理员是否为超管」。`*` 作为 admin_group.rules 的通配值，持有该值的分组的所有成员视作超管。

## ADDED Requirements

### Requirement: Permission check by rule name

系统 SHALL 提供按规则名（`admin_rule.name`）判定某管理员是否拥有该权限的能力。命中条件为：管理员属于超管分组、或管理员所在的任一未禁用分组持有该规则 id。

#### Scenario: Super admin wildcard
- **WHEN** 调用方传入管理员 uid 且该 uid 所属任一分组的 `admin_group.rules` 包含 `*`
- **THEN** 系统返回 true

#### Scenario: Rule owned via group
- **WHEN** 调用方传入管理员 uid 与规则名，且存在一条 `admin_rule` 记录的 `name` 与之相等、且该规则 id 出现在该管理员所属未禁用分组的 `rules` 字段中
- **THEN** 系统返回 true

#### Scenario: Rule not owned
- **WHEN** 调用方传入管理员 uid 与规则名，但管理员不在任何持有该规则 id 的未禁用分组中
- **THEN** 系统返回 false

#### Scenario: Disabled admin
- **WHEN** 调用方传入管理员 uid，但该 `admin` 记录的 `status` 为 0
- **THEN** 系统返回 false

### Requirement: Get user's full rule list

系统 SHALL 提供获取某管理员拥有的全部 `AdminRule` 记录的能力，返回值为该管理员可访问的规则记录集合（去重），按 `weigh` 升序、`id` 升序稳定排序。

#### Scenario: Super admin sees all enabled rules
- **WHEN** 调用方传入超管 uid
- **THEN** 系统返回全部 `status=1` 且未软删的 `admin_rule` 记录

#### Scenario: Regular admin sees union of group rules
- **WHEN** 调用方传入普通管理员 uid，且其所属未禁用分组的 `rules` 字段为非空、逗号分隔的规则 id 列表
- **THEN** 系统返回这些 id 对应的 `admin_rule` 记录（去重）

#### Scenario: Admin with no group memberships
- **WHEN** 调用方传入不属于任何分组的普通管理员 uid
- **THEN** 系统返回空切片

### Requirement: Get user's groups

系统 SHALL 提供获取某管理员所属全部未禁用分组的能力，返回值为该管理员所属的 `AdminGroup` 记录集合。

#### Scenario: Admin in multiple groups
- **WHEN** 调用方传入属于多个分组的普通管理员 uid
- **THEN** 系统返回这多个分组的 `AdminGroup` 记录

#### Scenario: Disabled group excluded
- **WHEN** 调用方传入管理员 uid，且其所属某分组的 `status` 为 0
- **THEN** 系统返回的集合中不包含该分组

#### Scenario: Admin with no groups
- **WHEN** 调用方传入不属于任何分组的管理员 uid
- **THEN** 系统返回空切片

### Requirement: Get user's rule IDs

系统 SHALL 提供获取某管理员拥有的全部规则 id 列表的能力（去重，按 id 升序），用于构建路由 / 按钮可见性等仅需 id 的场景。

#### Scenario: Super admin receives all rule IDs
- **WHEN** 调用方传入超管 uid
- **THEN** 系统返回全部 `status=1` 且未软删 `admin_rule.id` 的升序切片

#### Scenario: Regular admin receives union of group rule IDs
- **WHEN** 调用方传入普通管理员 uid
- **THEN** 系统返回其所属未禁用分组 `rules` 字段解析后（去重、升序）的 id 切片

#### Scenario: Admin with empty rules field
- **WHEN** 调用方传入管理员 uid，且其所属分组的 `rules` 字段为空字符串
- **THEN** 系统返回空切片

### Requirement: Super admin detection

系统 SHALL 提供判定某管理员是否为超管的能力。超管定义：管理员所属任一分组的 `admin_group.rules` 包含 `*`。

#### Scenario: Any group with wildcard makes admin super
- **WHEN** 调用方传入管理员 uid，且该管理员所属任一分组的 `rules` 字段为 `*`
- **THEN** 系统返回 true

#### Scenario: Admin without wildcard groups
- **WHEN** 调用方传入管理员 uid，且其所属全部分组的 `rules` 字段均不包含 `*`
- **THEN** 系统返回 false

#### Scenario: Admin with no groups
- **WHEN** 调用方传入不属于任何分组的管理员 uid
- **THEN** 系统返回 false

### Requirement: rules field format

系统 SHALL 解析 `admin_group.rules` 字段的两种形式：
- 值为 `*` 时，视为通配，授予该分组全部规则权限；
- 值为空字符串时，视为无权限；
- 其他情况视为以英文逗号分隔的 `admin_rule.id` 列表，解析时跳过空段与非数字段。

#### Scenario: Wildcard literal
- **WHEN** 某分组的 `rules` 字段恰好等于 `*`
- **THEN** 该分组所有成员视作超管

#### Scenario: Comma-separated IDs
- **WHEN** 某分组的 `rules` 字段为 `"1,2,5"`
- **THEN** 该分组所有成员拥有 `admin_rule.id` 为 `1`、`2`、`5` 的规则

#### Scenario: Whitespace around IDs
- **WHEN** 某分组的 `rules` 字段为 `"1, 2 , 5"`（id 周围含空格）
- **THEN** 系统在解析时跳过空白，得到 id 集合 `{1, 2, 5}`

#### Scenario: Empty rules string
- **WHEN** 某分组的 `rules` 字段为空字符串
- **THEN** 该分组所有成员不拥有任何规则

### Requirement: Exclude soft-deleted records

系统 SHALL 在权限校验过程中排除被软删（`deleted_at IS NOT NULL`）的 admin、admin_group、admin_rule 记录。

#### Scenario: Soft-deleted group is invisible
- **WHEN** 管理员所属某分组已被软删
- **THEN** 该分组不出现在 GetGroups 返回集合中，且其规则不影响 Check / GetRules / GetRuleIds 结果

#### Scenario: Soft-deleted rule is invisible
- **WHEN** 某 `admin_rule` 已被软删
- **THEN** 该规则不出现在 GetRules 返回集合中，也不参与 IsSuperAdmin 之外任何规则 ID 计算

### Requirement: Exclude disabled records

系统 SHALL 在权限校验过程中排除 `status=0` 的 admin、admin_group 记录。`admin_rule.status=0` 的规则同样不参与结果。

#### Scenario: Disabled admin has no permissions
- **WHEN** 管理员 `admin.status` 为 0
- **THEN** 该管理员视为没有任何规则（Check 全部返回 false，IsSuperAdmin 返回 false）

#### Scenario: Disabled group excluded
- **WHEN** 管理员所属某分组 `admin_group.status` 为 0
- **THEN** 该分组不参与任何结果聚合

#### Scenario: Disabled rule excluded
- **WHEN** 某 `admin_rule.status` 为 0
- **THEN** 该规则不出现在 GetRules、GetRuleIds 中，按规则名匹配时 Check 返回 false