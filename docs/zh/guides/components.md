---
title: 组合行为与外观
description: 使用稳定的场景部件，独立选择外观与交互策略。
contentType: How-to
---

# 组合行为与外观

语言：[English](../../guides/components.md) | 简体中文

## 从带测试的组合开始

运行 `go run ./examples/picker`，阅读 [picker 示例](https://github.com/Tangerg/oolong/tree/main/examples/picker)。应用选择数据与产品行为；base 拥有选择、导航和动作；kit 提供外观；scene 拥有实际结构。

程序化修改使用控制器的类型化操作。例如，`base.NewButton(ButtonConfig)` 通过其元素暴露唯一激活动作，表单暴露值与提交操作。不要为了让动作可发现，在皮肤里再创建第二个语义动作。

应用给编辑器增加快捷键时，通过 `controller.SetKeys(bindings)` 安装绑定，并在每次应用动作或帮助投影中读取 `controller.Keys()`。父对象不要保留另一份可替换的映射。替换编辑器映射时，发送、取消、退出与对应提示必须一起改变。配置为 nil 的映射不提供应用绑定；组件仍可提供自身的默认编辑动作。使用 `ActionFor` 匹配完整的 `input.Key`，包含修饰键和按下、重复、释放状态，不能只对 `Key.Code` 分支。

## 回执归签发它的所有者

表单调用 `Submit(id)` 时传入不透明的 `base.SubmissionID`。同步成功返回 `scene.Applied`，已接受的异步任务返回 `scene.Started`。回调返回后，在所有者回合通过 `form.Settle(id, outcome)` 交付结果。其他表单、已被取代的提交以及仍在执行提交回调的请求都不能使用该回执结算。

编辑器的元素快照保存捕获时的文本；即使文档已修改，仍可用 `element.Text()` 读取旧内容。关联的业务数据以不透明的 `base.ElementID` 为键；只有共享签发该标识的 `EditSession` 的编辑器能用它操作元素。调用 `tabs.SetRule(glyph)` 会同时更新分隔线和它占用的空间，空字符串移除该行。

## 无屏回答表单

为一段输入会话创建一个 `kit.Prompter`，依次回答多张 `*base.Form` 时复用它，让为后续答案预读的字节始终保留在同一所有者中：

```go
prompter, err := kit.NewPrompter(kit.PrompterConfig{
    Input:  os.Stdin,
    Output: os.Stdout,
})
if err != nil {
    return err
}
for _, form := range forms {
    if _, err := prompter.Ask(form); err != nil {
        return err
    }
}
```

这里的 `forms` 是应用持有的有序 `[]*base.Form`；带外观的 `kit.Form` 通过 `Controller()` 提供已有所有者。字段提供自身的 `Ask` 和 `Reply` 行为，表单拥有校验回执与提交，prompter 不另存一份答案模型。[Form 示例](https://github.com/Tangerg/oolong/tree/main/examples/form) 展示了屏幕与文本流程如何共享同一组字段。

`PrompterConfig.MaxAnswerBytes` 是显式的传输字节预算。零值不限制大小；正值不计 LF 或 CRLF 行尾。最后一条未带换行的答案仍可被接受，末尾可选的 CR 也会移除。超长行会被丢弃至行尾，然后 `Ask` 返回 `kit.ErrAnswerTooLong`，下次调用可以读取下一条答案。尚未读到答案就遇到空输入时返回 `io.ErrUnexpectedEOF`；读取失败不会提交尚未完整读取的答案。提示输出失败时，对应答案保持未读。

串行调用 `Ask` 并保留 prompter 指针；复制它，或通过其他 reader 读取借入的输入流，会破坏缓冲字节的归属。回调中再次调用同一 prompter 的 `Ask` 会收到 `kit.ErrPromptActive`。两个流的关闭责任均由调用方承担。

## 装配实际部件

`scene.Group` 接收带 key 的 `scene.Item` 子项与布局函数。覆盖层通过 `scene.Group` 的 `GroupConfig.Overlay` 配置实际挂载，`GroupConfig.Modal` 配置模态边界。绘制、焦点、路由、生命周期和语义观察由这些相同部件决定。

创建一次并保留组件实例。重复调用 `Element()` 是安全的；每次绘制都重建控制器，会丢失身份与手势。在 `Prepare` 前的所有者阶段更新 group 成员，然后绘制稳定的结果树。

## 替换皮肤

保留 base 控制器，在未挂载元素上设置绘制或测量槽。使用 core 的 text、grid、layout 原子能力绘制传入的 `scene.Frame`。从控制器读取值与状态。绘制使用候选几何，事件通过 `scene.Event.Read` 读取到达帧投影。

标签和装饰文本使用 `line := text.Of(value, style)`，配合它的 `Width` 与 `Draw` 方法。控制字符过滤、字素边界和 tab stops 由同一文字投影解释。Tab 从该文字行自身的起点计算，移动或裁剪视图不会改变其列位置。`grid.View.Text` 是更底层的单元格写入能力，不展开 tab。

`text.Projector` 通过 `Row.Content` 提供与绘制相同的 `text.Line` 投影。交付完整标签时使用 `line.Display().Clone()`：过滤控制字符，保留 tab 和显式字素边界，并使返回的 spans 与生产者脱离共享。`Entry.Rows` 在水平裁剪后仍保留这份完整投影，避免调整宽度改变已有选择的文本身份。`Line.String()` 提供复制或匹配所需的字节；从这些字节重建几何会丢失显式边界。Tab 展开以及所有字节到列的转换由文字布局负责。

[Base 参考](../base/index.md)与 [kit 参考](../kit/index.md)定义两侧职责。[State 示例](https://github.com/Tangerg/oolong/tree/main/examples/state) 展示显式所有者更新与归一化，不保留竞争性的 accessor 存储。

若外观只属于某一次组合，在子关系的 `scene.Item.Paint` 上提供画法；移出子项后，元素自己的画法仍然有效。若一个控制器需要同时替换多个兄弟组的子项，父组可用 `CheckChildItems` 和 `SetChildItems` 对直属子组整体预检、一次安装。

`state` 示例包含四个自有草稿：提交时归一化，拒绝时保留草稿；另有 `ConfirmControl` 以同一领域所有者与外部重置动作共享答案，并演示动态 ThemeOwner 与 ThemeScope 组合。

## 使用终端图像和跨布局快捷键

在 Kitty 或 Ghostty 中，从仓库根目录运行 `go run ./examples/image`。按 `1` 看全图，按 `2`、`3` 看左半或右半，也可以在命令后传入自己的 PNG 路径。示例只传输一次 PNG，切换时只改变同一显示位置的源矩形。[图像示例](https://github.com/Tangerg/oolong/tree/main/examples/image)

本地终端可以分别开启图像探测和键盘增强：

```go
features := term.Features{
    Probe:    true,
    Keyboard: term.KeyboardCompatible,
}
```

把 `features` 传给 `program.ScreenConfig.Terminal` 或 `program.InlineConfig.Terminal`。启动时，一张 1 像素 PNG 的查询和设备属性请求共用一次往返。与请求 ID 匹配的成功回复确认可以直接传输 PNG。终端已回复设备属性却没有答复图像查询，或明确拒绝查询时，终端名称不能单独启用 Kitty 图像。探测未完成时仍可参考名称和环境提示。传图前检查 `runtime.Images().Protocol().Supports(graphics.Live)` 和 `runtime.Images().CellSize()`。控件无法显示图片时会显示 `Alt`。

`runtime.Images().Transmit(ctx, png)` 返回接纳错误，或一个缓冲通道；通道只交付一次 `graphics.UploadResult`。写端接纳不代表终端已经创建图片。应在 `program.Work.Run` 中读取结果，再由 UI 所有者上的 `Apply` 安装 `Resource`。即使任务结果交付被撤销，成功创建的资源仍须有明确的释放者；image 与 Mermaid 示例展示了完整生命周期。Context 只取消未完成的上传；已经返回的资源持续有效，直到主动释放、交出终端或关闭终端。

Oolong 发送 Kitty 图片编号 `I`，并接收终端分配的物理图片 ID `i`，不再以本地计数器选择物理 ID。请求关联有容量上限，同一进程内不同 Terminal 实例不复用编号；取消后保留关联，才能在迟到回复到达时删除那张已分配的图片。删除命令因写端背压被拒绝时，在写端重新有容量后重试。终端拒绝、匹配但格式错误的回复、输入输出失败、取消与响应超时都会明确结算上传结果。终端会话要求独占前台输入与输出；请求关联的作用域是当前进程，协议本身无法认证任意残留或外部字节。

`kit.Image.Source` 使用原始 PNG 的像素坐标，左上角包含、右下角不包含，例如 `image.Rect(16, 8, 48, 40)`。零值表示整张图。并排显示同一张图时，为每个位置指定独立的 `Placement` ID。主动释放资源前先移除其显示位置。交出终端后，原句柄会失效，收回终端后需要重新传输。[Kitty 图像规范](https://sw.kovidgoyal.net/kitty/graphics-protocol/)

`term.KeyboardCompatible` 请求无歧义按键和备用键码。数字小键盘可单独绑定，例如 `kp-enter` 优先于普通回车绑定。通过 `runtime.Environment().Keyboard()` 可读取终端回复的标志。用 `keymap.Map` 注册快捷键，在按下或重复时调用 `ActionFor` 或 `Matcher.Handle`。当前布局的显式绑定优先。例如切换到西里尔字母布局后，同一物理键仍可通过基础布局命中 `Ctrl+A`。编辑器使用终端提供的 `Key.Text` 输入实际文字。需要松开事件时添加 `input.KeyboardReportEvents`，需要关联文本时添加 `input.KeyboardReportText`。后者会按规范同时开启全键转义编码。[键盘示例](https://github.com/Tangerg/oolong/tree/main/examples/keys) · [Kitty 键盘规范](https://sw.kovidgoyal.net/kitty/keyboard-protocol/)

默认的 `input.XTermKeyboard` 方言接受 XTerm 精确的修饰 F3 序列，包括 Shift、Alt 和 Ctrl。有效的键盘标志回复确认 `input.KittyKeyboard` 协议家族，即使增强标志为零也一样；此时 `CSI ... R` 按光标报告处理。方言同时拥有共享功能键序列的修饰位解释：XTerm Meta 与 Kitty Super 使用同一位。`CSI u` 始终遵循其明确的 Kitty 语法。只要请求键盘增强或启动探测，终端接管阶段就发送一次异步键盘查询；SSH 与交接后的重新接管也使用同一机制。`Terminal.Keyboard()` 仍返回启动探测的功能快照，解码器观察到的协议家族不会改写这份快照。自定义解析器可以设置初始的 `input.ParserConfig.Keyboard` 或 `input.StreamConfig.Keyboard`。光标查询所有者必须使用 DEC 私有 CPR（`CSI ?6n`）：无私有标志的第 1 行光标回复，与传统修饰 F3 的字节完全相同。[XTerm 控制序列](https://invisible-island.net/xterm/ctlseqs/ctlseqs.html)

`grid.CursorReset` 和零值 `grid.CursorStyle` 请求终端特定的 DECSCUSR 重置。在 xterm 中它表示闪烁方块，不能据此恢复用户配置或进入程序时的样式。打开宿主时可用 `term.Config.RestoreCursor` 显式选择恢复目标。启动探测也查询当前光标样式；有效的 DECRQSS 回复成为交接和关闭时的恢复目标，未取得回复时使用配置目标。`ssh.Terminal.Open` 使用同一显式目标，不增加阻塞式启动探测。无效光标形状在终端取得前被拒绝，因此纠正配置后仍可使用 SSH 终端仅有的一次打开资格。[XTerm 光标控制](https://invisible-island.net/xterm/ctlseqs/ctlseqs.html)

剪贴板默认使用原生 OSC 52，在 tmux 内也一样。`TMUX` 和 `TERM` 不能证明 DCS 透传已启用。只有会话明确配置了透传时，才设置 `term.Features{Clipboard: clipboard.Config{TmuxPassthrough: true}}`；SSH 适配器将同一配置用于已接受的远端终端。应用经 tmux 原生复制需要它的 `set-clipboard on` 路径；透传则受独立的 `allow-passthrough` 策略控制。[tmux 剪贴板](https://github.com/tmux/tmux/wiki/Clipboard) · [tmux 透传](https://github.com/tmux/tmux/wiki/FAQ)

使用 `runtime.Clipboard().Read(ctx, receive)` 发起定向读取。接纳错误立即返回；回调在 UI 所有者上接收读取结果。自定义宿主实现 `program.ClipboardReader.ReadClipboard(context.Context) (string, error)`，可以在该工作线程调用中阻塞。调用方取消或 runtime 结束会撤销回调交付。普通括号粘贴仍是输入事件；OSC 52 回复只结算发起它的读取。

`editor.Paste(ctx, complete)` 捕获原文档、修订、光标、选择区和挂载身份。焦点移动不会把结果转交另一编辑器。成功读取的结果在原编辑位置改变或编辑器卸载后过期；未挂载编辑器只有在从未挂载过时才能发起读取。读取失败原样报告，不再检查编辑位置。接纳和完成错误均明确报告。`editor.PasteError()` 属于最近一次键盘请求，旧请求的完成不能覆盖新请求的拒绝或成功。过期的成功结果会被静默丢弃，包装或组合后的读取失败仍然可见。Editor、Text、Input 和 Composer 的默认呈现使用独立反馈行，并通过语义 `Description` 与 `StateInvalid` 暴露完整原因。程序粘贴直接返回自己的错误，不改写物理输入反馈。空剪贴板成功返回，不删除当前选择区。

OSC 52 没有请求标识。响应超时会在十秒后结束等待，但不能证明迟到回复可以安全地归属新请求。取消和终端交接遵守相同规则：旧回复被消费前一直返回 `clipboard.ErrPending`，重复读取不能绕过未结清的查询。直接使用 `clipboard.Channel` 时，发送回调必须提供原子接纳语义；已经部分发出的查询也属于已发送，必须保留接收其回复的义务。

在期限内接收的完整匹配回复即使超过剪贴板或解码器的容量限制，也会以 `clipboard.ErrResponse` 结算并释放该查询的线协议槽位。已取消的结果保持取消；未终止的帧或其他选择区的回复不能释放槽位。自定义解码宿主须将回复首字节与末字节的观察时间传给 `clipboard.Channel.Answer` 或 `clipboard.Channel.Reject`，完整超量 OSC 52 还需提供 `input.OSCRejection.Prefix`。首次观察必须晚于查询接纳，完整接收必须早于响应期限。`input.Stream` 跨传输读取保留这两个时刻，并自动完成回复路由。`input.Key.At` 和 `input.Mouse.At` 同样保留首次观察时间，等待 Escape 超时不会重写它。

Ghostty 支持这两套线协议。嵌入 libghostty 的宿主可以单独配置各项能力，Oolong 以当前连接为准。内嵌 SSH 服务端适配器可处理键盘输入，目前未提供实时图像宿主接口。多路复用器的图像透传需要传输层另行支持。[Ghostty 终端能力](https://ghostty.org/docs/features)
