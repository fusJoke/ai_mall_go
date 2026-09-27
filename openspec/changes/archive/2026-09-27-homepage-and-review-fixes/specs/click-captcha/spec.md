# Spec Delta

## ADDED Requirements

### Requirement: 噪声元素不得与任何正确答案同名

`composeImage` 在选定 `Length` 个正确答案后 SHALL 用正确答案名集合构建 `exclude` 参数，并将其传给噪声 `pickElems`，以确保最终图片中每一个「正确答案元素」在视觉上具备**唯一**实例；噪声名 MUST NOT 与任何已选定的正确答案名重复。

#### Scenario: 给定常见配置下，噪声与正确答案不相交

- **WHEN** 后端以「6 noise / 3 correct / 池 = 26 大写字母 + 20 icon ≈ 46 元素」的配置生成一道验证码
- **THEN** 噪声数组与正确答案数组交集 MUST 为空；任何一张图片都不会出现两个视觉相同的「正确答案元素」

#### Scenario: 排除后噪声池仍足够

- **WHEN** 正确答案已经用掉池中 K 个元素，噪声池 SIZE 仍 MUST ≥ `NoiseLength`，否则应直接报错而非回退到「允许噪声名与正确答案名重复」
