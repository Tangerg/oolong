---
title: 发布单个模块
description: 依据可独立解析的依赖审查单个不可变模块 tag。
contentType: How-to
---

# 发布单个模块

语言：[English](../../guides/releasing.md) | 简体中文

## 准备发布提交

从 `scripts/modules.sh --public` 选择公开模块。提交已审查源码、所需依赖版本，以及名为 `[module/vX.Y.Z]` 的 `CHANGELOG.md` 小节。发布工具不更新依赖版本、不代造提交，也不发布无关模块。

使用与 `origin/main` 一致的干净 `main`。已发布 tag 不可变。语义导入后缀与物理模块路径必须一致，工具据此推导 tag 前缀。

## 检查预演结果

```sh
scripts/release.sh ui 0.1.0
scripts/release.sh extensions/markdown 0.2.0
```

每条命令只选择指定模块。工具运行仓库门禁，在 `GOWORK=off` 下验证选定模块，并检查拟发布 API 版本。工作区成功不能掩盖未声明依赖或从未发布的依赖版本。

## 发布审查后的 tag

```sh
scripts/release.sh ui 0.1.0 --execute
```

执行只创建并推送已审查的一个 tag，然后验证其远端提交。失败不授权删除或移动 tag、强推或绕过发布路径。应先检查真实状态，再决定后续操作。固定工具版本与质量门禁见 [CONTRIBUTING.md](https://github.com/Tangerg/oolong/blob/main/CONTRIBUTING.md)。
