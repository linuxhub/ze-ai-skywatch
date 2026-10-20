# 泽 · 观天镜 | Ze AI SkyWatch

**用 AI 看懂进程与网络。**

项目仓库：[linuxhub/ze-ai-skywatch](https://github.com/linuxhub/ze-ai-skywatch)。应用显示名为「泽 · 观天镜」，英文名为 **Ze AI SkyWatch**，可执行文件及进程名为 `ZeAISkyWatch`。

使用 **MyGO 0.5.5 原生 Go UI** 构建的 macOS / Windows 进程与端口监控工具。无需 WebView、Bun 或前端构建。

## 下载

在 [GitHub Releases](https://github.com/linuxhub/ze-ai-skywatch/releases/latest) 下载已打包的版本：

- [macOS Apple Silicon（M 系列芯片）安装包](https://github.com/linuxhub/ze-ai-skywatch/releases/download/v0.4.5/ZeAISkyWatch-0.4.5-macOS-arm64.dmg)
- [Windows x64 便携版](https://github.com/linuxhub/ze-ai-skywatch/releases/download/v0.4.5/ZeAISkyWatch-0.4.5-Windows-x64.zip)

选择 Releases 中的 DMG 或 ZIP 附件；GitHub 自动生成的 Source code 压缩包是源码。Windows 包完整解压后运行 `ZeAISkyWatch.exe`。

## 运行

当前版本 **0.4.5**（新增独立「关于」页面，展示作者与可访问、可复制的 GitHub 仓库地址）。安装包：`build/darwin-arm64/ZeAISkyWatch 0.4.5 arm64.dmg`，打开后将应用拖入“应用程序”即可安装或替换旧版。

已打包的本机版本：`build/darwin-arm64/ZeAISkyWatch.app`，双击即可打开。

```bash
open "build/darwin-arm64/ZeAISkyWatch.app"
```

运行环境：macOS 13+，Apple Silicon 或 Intel。当前附带的构建产物为 Apple Silicon。

## Windows 便携版

Windows x64 包：`build/ZeAISkyWatch 0.4.5 Windows x64.zip`。适用于 Windows 10 / 11 的 Intel / AMD 64 位电脑，完整解压后双击 `ZeAISkyWatch.exe`；无需安装 Go 或 WebView2。

Windows 版已适配进程 CPU / 内存、进程详情、TCP 监听端口、TCP / UDP 连接快照、物理网卡整机网速、本地 IP 标注和 AI 分析。Windows 通过 IP Helper API 读取连接表，UDP 只能展示本地绑定。结束进程只支持强制结束，使用同一个 Windows 进程句柄核对启动时间、所属用户并执行，避免 PID 复用误操作。

逐进程／逐连接速率、流量排行、连接地图暂不支持，相关分析入口禁用；连接速率显示 `—`。程序未做 Windows 代码签名。已完成交叉编译和静态检查、共享逻辑测试及 macOS 回归，尚未完成 Windows 实机运行验证。

```bash
./scripts/build-windows.sh  # 生成 Windows x64 .exe 和经完整性校验的便携 ZIP，需要 Python 3
./scripts/build.sh -platform windows/amd64  # 仅构建，安装了 NSIS 时也会生成 Setup 安装程序
```

Windows 本机安装 Go 后，可执行 `go tool mygo build -platform windows/amd64`；`scripts/go.sh` 的无 Go 自动下载逻辑仅适用于 macOS。

以下功能细节以 macOS 版为准，Windows 差异见上方及 [Windows 包使用说明](docs/windows-package.txt)。

## 开发与构建

```bash
./scripts/dev.sh                    # 开发模式，修改 Go 文件后自动重启
./scripts/build.sh                  # 为当前 Mac 构建 .app 和 .dmg
./scripts/build.sh -platform darwin/universal  # 同时支持 Apple Silicon / Intel
./scripts/go.sh test ./...          # 单元测试、原生 UI 交互测试和本机采集集成测试
./scripts/go.sh vet ./...
./scripts/go.sh tool mygo vet       # 检查原生 UI 元素的构建生命周期
```

需要 Go 1.27.2 或更新版本。脚本优先使用已安装的 Go；没有时从 Go 官方下载固定版本到项目 `.tools/`，校验 SHA-256 后使用，不修改系统安装。首次开发／构建需要联网下载工具链和依赖。Go 缓存放在 `/tmp/ze-process-manager-go-*`。

也可以自行安装 Go，然后直接运行 `go tool mygo dev` 或 `go tool mygo build`。

## 已实现

- 独立「关于」页面：产品名称、版本、作者「泽泽」，支持打开和复制 GitHub 仓库地址。
- 系统概览：整机 CPU 使用率、进程数量、内存使用率、TCP 监听端点数量。
- CPU Top 10 与完整进程列表，支持进程名称／PID 搜索和列排序。
- CPU 差分采样，进程身份使用 PID + 启动时间；首次采样显示 `—`。
- TCP 监听列表，支持地址、端口、进程名称、PID 搜索，回环地址筛选。
- 进程／端口详情：路径、命令行、用户、线程数、启动时间。
- 在 Finder 中显示、复制路径／命令。
- 停止进程前确认，可选 SIGTERM / SIGKILL，只操作当前用户有权限停止的进程。
- 手动刷新、自动刷新、1 / 2 / 5 / 10 秒间隔，设置本地保存。
- 菜单栏 CPU 数值和快捷菜单、登录启动、登录启动时隐藏窗口。
- 跟随系统明暗外观，中文控件，原生键盘与无障碍交互。
- 进程／端口行末 AI 图标，一键解释用途、资源占用、端口风险与排查建议，支持取消、重试、复制结果和发送数据预览。
- 多提供商配置：OpenAI 兼容与 Anthropic 协议、自定义 Base URL 和 API 密钥、手动模型 ID、读取模型列表、自动选择与真实对话测试。
- 轻量网络监控：每 2 秒直接读取 macOS 物理网卡计数，展示上传／下载速率、本次监控累计流量与最多 3 分钟的内存趋势。
- 按需网络分析：进程／应用汇总排行、TCP／UDP／QUIC 连接的本地与远端端点、连接详情、IP／进程／端口搜索、按速率或累计流量排序。
- 连接地图：离线底图、上传／下载双色曲线；可选光纤光点动画，只重绘地图、不重建表格，窗口隐藏／最小化时暂停。
- 网络流量布局：大窗口将可用高度分给主分析区和连接列表，排行同步增高，趋势图保留 120 高度；小窗口保持内容最小高度并支持整页滚动。地图、连线与可点击节点共同等比例缩放，避免底图拉伸。
- 独立“网络连接”页面：TCP／UDP、本地与远端 IP／端口、TCP 状态、UDP 绑定／对端状态；支持协议、监听与绑定、有对端、状态、地址范围、已标注筛选及列排序。
- 本地 IP 标注簿：IPv4／IPv6 精确匹配、名称／分组／说明，可从连接行快捷添加，在连接表、流量分析和地图节点提示中复用；保留原有“开放端口”页面。

## 网络连接与 IP 标注

打开侧栏“网络连接”即可采集，不需要启动“网络流量”的详细分析。默认每 5 秒更新 socket 列表，本页独立启动一个 `nettop` 子进程，每 2 秒采集下载、上传速率和网卡。在状态与 IP 标注之间显示“↓ 下载”“↑ 上传”“网卡”，支持数值排序和网卡搜索；详情及复制信息也包含这三个字段。首次采样或无法匹配计数时显示 `—`，有效采样且无传输时显示 `0 B/s`；通配监听没有固定网卡，具体地址绑定可通过本地接口识别网卡。只按 PID、进程启动时间、协议及两端地址精确匹配流量，不把进程总速率分摊到连接。

暂停后保留表格并停止采集，仍可手动刷新（最多采集 8 秒以建立速率基线）；切换页面、隐藏或最小化窗口时停止本页 socket 与流量采集。返回页面后重新建立速率基线并恢复自动采集（手动暂停的状态保持到本次运行结束）。本页刷新独立于常规进程自动刷新开关。较窄窗口可横向滚动表格查看右侧列。

列名采用“本地／远端”，不推断连接发起方向。TCP 监听无远端；UDP 无 TCP 连接状态，显示“已绑定”“未绑定”或“有对端”。地址范围优先按远端分类，无远端时按本地地址分类；通配地址不代表可以从公网访问。列表是系统可见 socket 的定时快照，短连接可能遗漏；普通用户权限下其他用户或受保护进程可能不可见。

选中连接并打开“连接端点详情”，可点击“查看进程”进入同一进程的详情（路径、命令行、用户、资源占用及 AI 分析），通过“返回连接详情”回到原连接；关闭或按 Escape 也保留原连接上下文。可直接点击“结束此进程…”或在进程详情中点击“停止进程…”，两者共用确认流程，默认发送 SIGTERM，可勾选 SIGKILL。确认对象固定为所选进程的 PID 和启动时间；执行前重新校验身份及当前用户权限，防止 PID 复用误操作。停止会影响该进程的全部连接与服务，成功发送信号后立即刷新连接、进程和端口列表；取消或失败则保留原详情。身份未确认的连接不能发起进程操作。

在“设置”右上角打开“IP 标注簿”，添加 IP、名称和可选分组／说明；也可选中连接后点击“标注远端 IP”。例如将 `121.43.212.142` 标为“生产环境官网”，所有相同远端 IP 的连接会立即显示紫色标注。点击编辑可修改或确认删除。同一 IP 只能有一条标注，IPv6 等价写法及 IPv4 映射地址会规范化匹配；不支持域名、端口或 CIDR 网段规则。

IP 标注簿列表随窗口高度撑开，搜索栏、表头和底部数量保持固定，记录在列表内滚动。编辑时，“标注分组”下方自动显示已保存记录中的分组标签，去除重复与空白，按使用数量优先排列；点击即可填入并高亮选中，支持清空或手动输入新分组。新分组在标注保存成功后可供后续选用，不修改现有标注数据格式。

标注在本机 `settings.json` 保存，不需要联网位置查询，也不会主动上传标注。保存失败会保留原记录。每次采集最多展示 16384 条连接，超出时提示；采集失败保留上次成功表格和时间，并显示错误。

## 网络与性能

未打开“网络连接”且未开启详细分析时，不启动 `nettop`，也不发起 IP 查询。物理网卡统计只计算 macOS 的 `en*` 网卡（Wi-Fi／以太网），排除回环、VPN 虚拟网卡与桥接层，避免直接叠加这些层造成重复统计；它包含局域网流量，并不等于运营商计费流量。接口首次出现、计数器重置时建立新基线，不把历史累计当作瞬时速度。

“网络流量 → 开始分析”或“设置 → 网络与性能 → 启用详细网络分析”会启动一个持续运行的系统 `nettop` 子进程，每 2 秒采样。关闭时取消子进程、位置查询并释放明细缓存；重启应用总是回到轻量模式。动画与联网位置查询是独立且默认关闭的持久设置。

维护注意：持续 CSV 必须包含 `-J time,interface,state,bytes_in,bytes_out`，按数据行的原始采样时钟计算字节差分，并处理跨午夜。不能用程序读取管道的时间作为采样时间：缓冲输出、较慢的进程路径解析都可能使多轮记录紧接着被读入，造成瞬时速率虚高。`TestBufferedNettopFramesUseSourceTime` 专门覆盖批量回放场景。

本机 Apple M3 的 100 次微基准中，单次物理网卡读取约 70 微秒；这只代表采集函数，不代表应用整体 CPU 占用。地图底图只生成一次缓存纹理，光点通过 `Painter.After` 限制到约 30 帧且仅请求重绘；`TestNetworkAnimationOnlyRepaintsAndPausesWhenHidden` 验证动画不重建表格，并在隐藏时停止。

- 进程身份使用 PID + 启动时间；能读取应用路径时将辅助进程汇总到外层 `.app`。无法验证身份的记录不会归给其他进程。
- 进程累计从本次分析的首个可见样本建立基线，只累计后续正向差值；消失后再出现的记录重新建立速率基线。无法观测的短连接可能遗漏，不能替代完整流量审计。
- 表格通常比实时传输落后一轮采样。进程统计包含回环／虚拟网络，与物理网卡总量口径不同；代理进程的外部连接不能可靠还原到原始应用请求。
- 每轮最多保留 4096 个进程和 8192 个端点，分析期间保留至多 4096 个进程身份；地图最多绘制 20 个目的 IP。同进程、同协议和同两端地址的 socket 合并，控制长会话内存与绘图开销。
- “联网查询 IP 位置与组织”开启后，向 [ipwho.is](https://ipwhois.io/documentation) 发送公网目标 IP，不发送本地地址、进程名、URL 或请求内容。查询串行限速，每次分析最多查询 200 个 IP；成功和失败均缓存到本次分析结束。查询服务的可用性、限额与结果准确性由服务方决定。
- 地图节点位置是估算；无位置、私网和本机节点放在示意区域。本机节点的位置为示意，连线不代表实际光缆／路由，光点不对应逐个真实数据包。HTTPS 请求内容与具体 URL 不在采集范围内。

## AI 分析

1. 打开“设置 → AI 模型服务 → 添加提供商”，选择预设或填写自定义服务。
2. 配置名称、API 协议、Base URL、API 密钥。Base URL 通常包含 `/v1`，例如 `https://api.openai.com/v1`；本机服务允许 `http://localhost:端口/v1`，其他地址要求 HTTPS。
3. 手动填写服务支持的模型 ID（例如 `gpt-5.6`），或读取模型列表后选择。可启用自动选择：优先使用填写且在列表中的模型，否则按内置规则选择文本模型；列表不可用时尝试手动填写的模型。这不代表模型质量排名，实际可用性以服务返回和连接测试为准。
4. 点击“测试连接”。这会发送一条简短对话，显示实际调用的模型和耗时，可能产生少量 API 用量。保存后可将提供商设为默认。
5. 点击概览、进程或端口列表右侧的紫色 AI 图标，自动打开分析面板。普通详情中的“AI 分析”页也可手动开始分析。

仅主动请求分析时才向默认模型服务发送所选进程的摘要：名称、PID、启动时间、可执行路径、CPU、RSS、线程与匹配同一启动实例的 TCP 监听端点。路径中的 `/Users/用户名` 会隐藏；不发送命令行、环境变量、文件内容或其他进程。面板内可查看本次发送的 JSON；分析结果仅保存在当前详情会话中。

API 密钥随设置保存在本机 `settings.json`，文件权限为 `0600`，未加密。模型结果不会触发命令或自动停止进程；未知字段以 `null` 提供，单次采样和模型推断不能代替持续监控或安全检测。关闭面板、切换目标或取消分析会取消旧请求，旧结果不会覆盖新进程。

接口参考：[OpenAI Chat Completions](https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create)、[Anthropic Messages](https://platform.claude.com/docs/en/api/messages/create)。

## 使用说明

- 在表格中单击选择，再点击“详情”；双击行或按 Enter 也可打开详情。
- 进程 CPU 的 100% 对应一个逻辑核心，因此可能超过 100%。整机 CPU 是 0–100% 的口径。
- 进程内存是 RSS，不等同于活动监视器的内存 footprint。
- CPU / RSS 通过 `proc_pidinfo(PROC_PIDTASKINFO)` 一次读取，并核对返回字节数。读取失败显示不可用，不伪装成 0%。
- 整机 CPU / 内存、按需进程详情使用 gopsutil；进程身份批量读取 macOS sysctl。动态库连接复用，避免对每个进程反复加载系统库。
- 端口通过系统 `/usr/sbin/lsof -nP -iTCP -sTCP:LISTEN -FpcfnPt` 采集。
- 端口扫描间隔至少 5 秒；扫描失败保留上次成功结果、原始时间和错误提示。
- 监听端点按 PID、协议、地址族、地址、端口计数；同一服务的 IPv4 / IPv6 可能显示两行。
- `127.0.0.1` / `::1` 仅供本机访问；通配或网卡地址是否可远程访问还取决于防火墙与网络。
- “开放端口”页面只显示 TCP 监听端口；网络分析另行显示可见的 TCP／UDP／QUIC 对端。历史曲线磁盘存储、管理员提权不在此版本范围内。
- 普通权限可能无法读取其他用户或受保护进程。App 不要求 root，不会绕过系统限制。
- “停止进程”影响其全部服务；端口可能被 launchd、Docker 或其他管理程序重新启动。
- 启用菜单栏图标时，关闭主窗口会继续在后台采集；从菜单栏重新打开，⌘Q 完全退出。
- 登录启动设置需在打包的 `.app` 中使用，安装到“应用程序”后设置较为合适。此项目默认不开启登录启动。

设置文件保存在原有应用数据目录下的 `settings.json`，改名后继续沿用，保留已有模型配置与 IP 标注。macOS 正式版为 `~/Library/Application Support/观天镜/`，Windows 正式版为 `%AppData%\观天镜\`；开发版本使用 `观天镜 Dev` 目录独立保存。应用标识继续沿用 `dev.zepinglai.macmonitor`。

## 项目结构

```text
main.go                         应用生命周期、采样调度、菜单栏
views.go                        概览、进程、端口、设置、详情弹窗
about_views.go                  独立关于页、作者与 GitHub 仓库入口
internal/monitor/               CPU 差分、进程身份、macOS 原生资源读取
internal/ports/                 lsof 采集和字段解析
internal/connections/           独立 TCP／UDP socket 快照与状态解析
internal/ipnotes/               IP 标注校验与地址规范化
internal/network/               原生网卡计数、nettop 流量差分、可选 IP 位置查询
internal/actions/               停止进程与用户／身份校验
internal/ai/                    模型服务、模型发现、分析提示与 HTTP 请求
internal/settings/              本地 JSON 设置、原子写入
ai_app.go / ai_views.go          AI 任务生命周期、提供商配置与分析界面
network_app.go / network_views.go / network_map.go  按需网络分析生命周期、表格、缓存地图与动画
connections_app.go / connections_views.go          独立连接采集生命周期、筛选与详情
ipnotes_app.go / ipnotes_views.go                   IP 标注管理、即时匹配与设置界面
resources/                      应用与菜单栏图标
scripts/                        Go 工具链、开发和打包入口
```

## 验证

测试会创建自己拥有的临时 TCP 监听器和 `sleep` 子进程，验证扫描、PID 身份检查和停止功能，不会停止用户的其他进程。进程采集及端口测试需要在允许 sysctl / 本机端口的普通 macOS 环境执行；限制型执行沙箱可能拒绝它们。

```bash
MAC_MONITOR_SCREENSHOTS=/tmp/mac-monitor-previews ./scripts/go.sh test -run TestRenderLayouts
./scripts/go.sh test ./internal/monitor -run '^$' -bench BenchmarkSample -benchtime=3x
NETWORK_LIVE_TEST=1 ./scripts/go.sh test ./internal/network -run TestStreamOwnedConnectionCancellation -count=1
./scripts/go.sh test ./internal/network -run '^$' -bench BenchmarkPhysicalCounters -benchmem
```

截图测试使用明确的固定测试数据，网络流量页面覆盖明暗外观与 1180×700 至 2560×1380 窗口，并检查窗口缩放、整页滚动及地图节点点击。

## 分发

构建默认使用本机 ad hoc 签名，可在构建机器上运行。对外分发前需要配置 Apple Developer ID 签名与公证。参见 [MyGO 分发文档](https://mygo.egoist.dev/docs/distribution)。
