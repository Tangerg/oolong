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

所有检查都在已捕获提交的临时 checkout 中运行。编辑原工作区不会改变正在检查和打标签的候选代码。检查若修改候选代码，发布会中止。无论成功或失败，临时 checkout 都会被清理。

## 发布审查后的 tag

```sh
scripts/release.sh ui 0.1.0 --execute
```

执行只创建并推送已审查的一个 tag，然后验证其远端提交。失败不授权删除或移动 tag、强推或绕过发布路径。应先检查真实状态，再决定后续操作。固定工具版本与质量门禁见 [CONTRIBUTING.md](https://github.com/Tangerg/oolong/blob/main/CONTRIBUTING.md)。

要从已有标签查找公开模块目录，运行 `scripts/module-tag.sh --resolve TAG`。它按正向规则验证归属，也适用于存放在独立子目录中的主版本。每个模块使用自己的迁移记录：已发布 API 在开发 workspace 之外导出，当前 API 在本仓 workspace 中导出。

推送标签后，CI 在检出目录之外创建空的消费模块，按标签的精确版本获取源码并使用解析到的依赖构建。手动验证拟发布版本时使用检出的源码。
