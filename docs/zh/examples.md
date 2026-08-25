---
title: 从可运行示例逐步学习
description: 通过十五个经过测试的 Oolong 命令，从核心运行时学习到有界模拟 Agent。
contentType: Tutorial
---

# 从可运行示例逐步学习

语言：[English](../examples.md) | 简体中文

这些示例组成一条可执行学习路径。请在仓库检出中运行示例，再阅读相邻测试，了解每个切片保证的公开行为。

## 运行并测试示例目录

在仓库根目录运行一个命令，`go.work` 会连接当前模块：

```sh
go run ./examples/hello
```

在示例模块中测试所有命令：

```sh
cd examples
go test ./...
```

如果你还不熟悉运行时契约，请从第一组开始。已经掌握前置知识时，可以直接进入后面的分组。

## 学习运行时契约

这些示例依次引入绘制、输入所有权、按键序列和 headless 字段：

| 示例 | 运行 | 操作 | 重点 |
| --- | --- | --- | --- |
| [`hello`](https://github.com/Tangerg/oolong/tree/main/examples/hello) | `go run ./examples/hello` | 任意键计数；`q` 或 `Ctrl+C` 退出 | `program.Component`、`Config.Root`、`grid.View` |
| [`keys`](https://github.com/Tangerg/oolong/tree/main/examples/keys) | `go run ./examples/keys` | `g` 延时移动；`gg` 跳转；`q` 退出 | 具名 action、精确前缀序列、调用方超时 |
| [`form`](https://github.com/Tangerg/oolong/tree/main/examples/form) | `go run ./examples/form` | `Tab` 换字段；方向键选择；`Enter` 提交 | 受控字段、验证、网格和语音渲染 |
| [`state`](https://github.com/Tangerg/oolong/tree/main/examples/state) | `go run ./examples/state` | 在四个字段中输入；`Tab` 前进；`Enter` 提交 | 通过同一条 `Accessor` 接缝表达本地状态、精确绑定、归一化与拒绝 |

通过管道运行表单，可以在没有终端时使用同一组 headless 字段：

```sh
go run ./examples/form | cat
```

修改这些命令之前，请阅读[构建第一个界面](getting-started.md)。

## 组合 headless 行为

这些示例组合可复用行为，同时让应用保留产品布局与含义：

| 示例 | 运行 | 操作 | 重点 |
| --- | --- | --- | --- |
| [`picker`](https://github.com/Tangerg/oolong/tree/main/examples/picker) | `go run ./examples/picker` | 输入以过滤；方向键移动；`Enter` 选择 | 文本字段、模糊排序、列表、匹配高亮 |
| [`composer`](https://github.com/Tangerg/oolong/tree/main/examples/composer) | `go run ./examples/composer` | 输入 `@`；方向键选择；`Enter` 提交 | 补全、草稿历史、原子粘贴元素 |
| [`files`](https://github.com/Tangerg/oolong/tree/main/examples/files) | `go run ./examples/files .` | `Tab` 换栏；方向键导航；`q` 退出 | 焦点、树身份、viewport、指针路由 |
| [`dashboard`](https://github.com/Tangerg/oolong/tree/main/examples/dashboard) | `go run ./examples/dashboard` | 按 `1`–`3`、排序表头、调整滑块 | 调用方拥有的标签与滑块、表格、进度、动画生命周期 |

库不会在已有原语之外添加第二套 `Picker` 或 `FileBrowser` API。每种产品交互都组合自同一批控制器。

请阅读[组合一个可换主题的选择器](components.md)，理解本组示例使用的所有权与外观接缝。

## 渲染可选内容

这些示例先证明每个内容模块都能独立工作，再展示组合方式：

| 示例 | 运行 | 操作 | 重点 |
| --- | --- | --- | --- |
| [`markdown`](https://github.com/Tangerg/oolong/tree/main/examples/markdown) | `go run ./examples/markdown` | `q` 退出 | 完成块、测量、宽度相关布局 |
| [`latex`](https://github.com/Tangerg/oolong/tree/main/examples/latex) | `go run ./examples/latex` | `q` 退出 | 独立公式、二维文本布局 |
| [`content`](https://github.com/Tangerg/oolong/tree/main/examples/content) | `go run ./examples/content` | `q` 退出 | Markdown 组合 Highlight 与 LaTeX 平级渲染器 |

请阅读[渲染 Markdown、代码与数学公式](content.md)，使用三个自然入口及消费方拥有的组合接缝。

## 发布流式输出

这些示例添加后台工作、增量变换、内联发布和应用策略：

| 示例 | 运行 | 操作 | 重点 |
| --- | --- | --- | --- |
| [`run`](https://github.com/Tangerg/oolong/tree/main/examples/run) | `go run ./examples/run -- go test ./core/...` | `Ctrl+E` 编辑；`Ctrl+Z` 暂停；`Ctrl+C` 退出 | ANSI 解码、子进程输出、终端交接 |
| [`read`](https://github.com/Tangerg/oolong/tree/main/examples/read) | `go run ./examples/read` | 回答结束后按 `Ctrl+C` 退出 | 增量 Markdown、扩展、稳定块、开放尾部 |
| [`streaming`](https://github.com/Tangerg/oolong/tree/main/examples/streaming) | `go run ./examples/streaming` | `Enter` 允许；`Ctrl+X` 取消 | 受控对话框、有界摄入、transcript、失败、resize |
| [`agent`](https://github.com/Tangerg/oolong/tree/main/examples/agent) | `go run ./examples/agent` | `Enter` 发送；`/help` 列出命令 | 计划、补全、工具审核、diff、有界历史 |

`streaming` 是规范库集成。`agent` 添加产品策略，但不会把模型名称、工具语法或工作区效果放进框架。两个命令都不会发起网络请求或修改文件。

请先阅读[构建有界流式输出](streaming.md)，再阅读[构建有界 Agent 界面](agent.md)以理解高级边界。

## 审核测试与视觉 golden

每个 `main_test.go` 都通过 `programtest.Host` 启动命令、发送事件并断言可见行为。流式示例还会在真实伪终端 (PTY) 上测试终端回滚区和空闲输出。

Agent 命令保存窄屏与宽屏文本 golden。只有新几何确实符合预期时才重新生成：

```sh
cd examples
go test -update ./agent
```

请把每个变化的 golden 与代码修改一起审核。普通 `go test ./...` 永远不会重写 fixture。
