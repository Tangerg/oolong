---
title: 资源与发布生命周期
description: 区分执行、交付、准入和终端物理完成。
contentType: Reference
---

# 资源与发布生命周期

语言：[English](../../core/lifetimes.md) | 简体中文

## 作用域工作

UI 所有者通过 `Runtime.NewScope` 创建 `Scope`。`ScopeConfig.Attachment` 可将交付资格绑定到一个挂载代次，`Context` 增加外部生命周期。`Runtime.Start(scope, Work[T]{Run, Apply})` 在启动 `Run` 前预留完成容量。工作线程返回类型化结果；只有作用域仍具资格时，`Apply` 才在 UI 所有者上执行。

`TaskStatus` 将工作线程结果、`CancelRequested` 与结果交付分开。调用 `Cancel` 是请求协作，不能声称外部工作已经结束。视图卸载后，即使工作成功，UI 交付也可能被撤销。`Task.Done` 和 `Task.Settled` 暴露不同边界。普通 dispatcher 拥塞不会丢弃已接受任务的完成结果。

## 永久发布

`Publisher.TryPublish` 在接受责任前，将完整 drawable 捕获为不可变、自有的单元格。原生 painter 被拒绝，因为其生命周期需要活动帧所有权。捕获失败、不支持的输出与容量拒绝，都将责任保留给调用方。空捕获内容获得 sequence 为零的 written 空操作回执。

| 回执状态 | 含义 |
| --- | --- |
| `Accepted` | 发布器拥有不可变内容，但可能仍未进入写入器 |
| `Written` | 物理写入水位已覆盖对应 sequence，或贡献本身为空 |
| `Failed` | 责任因已知失败而结束 |
| `UnknownPartial` | 已进入写入器的字节可能部分到达目的地 |

发布 ID 标识已接受责任，不替代传输 sequence。待合成与已进入写入器的捕获共享 item 和字节预算，经过一个缓慢队列不会提前释放该预算。

## 组合的只读借用

`Element.ReadOnly` 同步执行投影，期间由组合的阶段所有者拒绝模型与挂载关系的变更。已挂载组合使用场景根；尚未挂载的组合使用最外层 Group 或 Element。借用期间禁止挂载或移动组合，因此回调无法切换到另一个所有者来绕过检查。嵌套借用会恢复外层阶段，panic 或 goroutine 退出也遵循同样的释放规则。

Transcript 在发布捕获时复用这个原子能力。它的滚动、选择与 sticky 控制器已经共享同一个 Element，因此各自的命令都查询同一个阶段所有者，不再维护额外的捕获标志。`Element.Measure` 同样复用该边界。这是 UI 所有者顺序上的合同，不能据此从工作线程并发调用 UI 操作。

控制器的绘制和 staging 必须使用挂载其真实 Element 的 Root 所提供的帧。自定义控件在计算候选几何或调用渲染回调前，调用 `Element.CheckProjection(frame)`。应挂载这个 Element，并在挂载前填充它的 paint slot；另一层无关包装不能成为第二个几何生产者。尚未挂载的控制器仍支持模型操作与测量，但不能提交几何。没有关联 Element 的独立原子能力遵循其帧的阶段。

只读借用也拒绝 snapshot 的 staging 与 refinement。一个候选必须先 commit 或 abort，才能开始新的只读借用或测量。这样，之前保存的候选就不能在回调内部改变正在观察的几何，同时 Commit 仍可以结算已接纳输出，不增加一个新的拒绝点。

`ScrollLayout` 只在 Prepare 活动期间读取和细化同一个 staged candidate，不保留可以独立推进的 offset 副本。细化被拒绝时，候选值及可读 offset 都保持不变；准备结束后，通过 `Scroll.Offset` 读取控制器当前的位置。

## 原子输出与关闭

`Publisher.Prepare` 读取不可变待发前缀。`grid.Inline.Prepare` 将它与活动帧一起合成，但不推进锚点。运行时验证完整编码载荷与写入器容量，再连续提交发布、输出和场景候选，中间不运行应用回调。拒绝时中止候选，之前的完整投影继续有效。

`CloseAdmission` 停止新发布，同时保留已接受内容用于最后一帧和排空。`Written` 结算真实进度。终端失败以实际原因结算剩余责任。延迟的 `FlushNotifications` 在全部所有者结算之后执行，使 Transcript 在后续所有者轮次恢复被容量阻塞的发布。

Inline 关闭时先等待最后一帧写完，再接纳光标清理，并等待清理本身完成。因此，即使写入器只允许一个普通帧，也能保持输出顺序与光标恢复，不依赖后台写入线程恰好先获得调度。

## 独立资源预算

StreamDocument 限制保留源字节；Capture 限制单元格与自有字符串；Publisher 限制完整保留捕获。运行时限制动作与资源准入，写入器限制编码字节与待写传输。这些数量不会被拼成虚构的通用内存数字；自定义渲染器内部的调用方分配仍由调用方负责。
