# 主题切换功能规划（光明 / 黑暗 / 赛博朋克）

## 一、概述

为 Xx1ServerManager2 增加主题切换：**光明（现有）**、**黑暗（暗色版）**、**赛博朋克（定制视觉）** 三种主题。
主题状态持久化（localStorage + 设置库双重保存），切换即时生效，图表与 SSH 终端的配色随主题联动。

## 二、现状分析（Phase 1 探索结论）

- **变量体系**：[style.css](file:///f:/forProgram/INDEXxiaoxia/GUI/Xx1ServerManager2/prj/frontend/src/style.css) 的 `:root` 已定义完整「纸感浅色」语义变量（`--paper/--card/--ink/--blue/--pink/--green/--red/--yellow/--violet` + 全套 `--el-*` 组件变量）。大部分页面已使用这些变量 —— 这是三主题切换的最大优势：**dark/cyber 只需覆盖变量**，绝大多数 UI 自动换肤。
- **设置入口已存在**：设置页[settings/Index.vue](file:///f:/forProgram/INDEXxiaoxia/GUI/Xx1ServerManager2/prj/frontend/src/views/settings/Index.vue) 已有主题下拉（浅色/深色，`settings.theme` 存数据库），但**从未应用生效**。
- **硬编码色值（需迁移为变量）**：
  - [Layout.vue](file:///f:/forProgram/INDEXxiaoxia/GUI/Xx1ServerManager2/prj/frontend/src/views/Layout.vue)：侧栏 `#FBFAF8`、hover `#F1EFEA`、顶栏 `#FFFFFF`/`rgba(255,255,255,.72)`
  - [ServerSelector.vue](file:///f:/forProgram/INDEXxiaoxia/GUI/Xx1ServerManager2/prj/frontend/src/components/ServerSelector.vue)：树背景 `#FBFAF8`
  - [Execute.vue](file:///f:/forProgram/INDEXxiaoxia/GUI/Xx1ServerManager2/prj/frontend/src/views/commands/Execute.vue) / [Upload.vue](file:///f:/forProgram/INDEXxiaoxia/GUI/Xx1ServerManager2/prj/frontend/src/views/files/Upload.vue)：面板底 `#FBFAF8`、输出块 `#23262D/#C7CBD4/#E5979D`、组色兜底 `#C4C6CE`
  - [services/Index.vue](file:///f:/forProgram/INDEXxiaoxia/GUI/Xx1ServerManager2/prj/frontend/src/views/services/Index.vue)：服务状态文本块 `#23262D/#C7CBD4`
  - [terminal/Index.vue](file:///f:/forProgram/INDEXxiaoxia/GUI/Xx1ServerManager2/prj/frontend/src/views/terminal/Index.vue)：整页深色调 + xterm theme 写死
  - [dashboard/Index.vue](file:///f:/forProgram/INDEXxiaoxia/GUI/Xx1ServerManager2/prj/frontend/src/views/dashboard/Index.vue)：统计卡渐变终止色、进度条色
  - [style.css](file:///f:/forProgram/INDEXxiaoxia/GUI/Xx1ServerManager2/prj/frontend/src/style.css) 自身：滚动条、光斑、卡片边框/阴影 rgba
  - [Unlock.vue](file:///f:/forProgram/INDEXxiaoxia/GUI/Xx1ServerManager2/prj/frontend/src/views/Unlock.vue)：登录页光斑 rgba、阴影
  - [presets/Index.vue](file:///f:/forProgram/INDEXxiaoxia/GUI/Xx1ServerManager2/prj/frontend/src/views/presets/Index.vue)：空态边框 `#F0D2D5`
- **图表**：[Monitor.vue](file:///f:/forProgram/INDEXxiaoxia/GUI/Xx1ServerManager2/prj/frontend/src/views/servers/Monitor.vue) series 色写死，**未配置坐标轴文字/网格线颜色** —— 暗背景下标尺将不可见，必须联动。
- **分组颜色色板**（groups/Index.vue）是用户数据，不随主题变。

## 三、赛博朋克风格设计规范（重点）

> 提炼本质：**暗底霓虹、玻璃面板、发光描边**。不堆砌全屏故障特效以保证可读性与流畅度，用克制的「特效层」做出质感。

### 3.1 基调
- **底色**：近黑深藏青，由 `#05070D → #0B0F1A` 垂直渐变（非纯黑，带蓝黑氛围）。页面四角暗部压深。
- **双霓虹色**：
  - 主色 青蓝（Cyan）`#21E6FF` / 深一档 `#0B8EF0`
  - 辅色 洋红紫（Magenta）`#FF3EA5` / `#B14EFF`
  - 语义：成功=霓虹绿 `#00FFA3`，警告=琥珀 `#FFB020`，危险=霓虹红 `#FF4D6D`
- **高对比低亮度**：文字主灰白 `#E8EDFF`，次级 `#8A94B8`，弱化 `#5C6485`；背景元件亮度整体压暗。

### 3.2 背景特效层（CSS，低性能成本）
1. **科技网格**：页面背景叠加两层 1px 线性渐变网格（`linear-gradient`，间距 36px，色 `rgba(33,230,255,.05)`），随页面滚动。
2. **微噪点**：固定 `feTurbulence` SVG data-URI 噪点层覆盖全屏（`opacity .03`，静态图片无动画成本），消除纯色平涂感。
3. **氛围光晕**：右上角青色 radial 光斑 + 左下角洋红光斑（沿用现有 `#app::before/::after` 结构改色，保持 `breathe` 呼吸动画）。
4. **数据流**：全屏顶部一条 1px 青色光带缓慢下移（`background-position` 动画 10s 循环，`opacity .08`），模拟数据流；`prefers-reduced-motion` 下禁用。
5. 顶部标题栏/侧栏：玻璃拟态（`backdrop-filter: blur(14px)` + 半透明深底），下方 1px 青蓝微光分隔线。

### 3.3 材质与边框
- **面板（card/对话框/下拉/表格容器）**：`rgba(13,18,32,.62)` 半透明底 + 1px 描边 `rgba(33,230,255,.22)` + 外发光 `box-shadow: 0 0 12px -6px rgba(33,230,255,.5), 0 8px 30px rgba(0,0,0,.45)`；**仅 header/aside/卡片用 blur 玻璃**，列表内部不再 blur（性能）。
- **霓虹发光**：主按钮/激活菜单项/进度条 = 渐变填充 + 外发光；激活菜单项加 2px 发光左条（青色）；输入框聚焦 1px 青色描边 + 3px 发光环。
- **表格**：表头半透深底 + 青色微光；行 hover 淡青 `rgba(33,230,255,.06)`；斑马纹同系微变。

### 3.4 极简控件与质感
- 控件沿用 Element Plus 圆角体系，仅改色；分页/标签为细霓虹边框小胶囊。
- 标题/数字：青-洋红双层 `text-shadow` 微发光（`0 0 12px rgba(33,230,255,.35)`）。
- **轻微故障感**（点缀，不滥用）：激活 tab 顶部滑块用 `clip-path` 位移随机帧动画（帧抖 3px，0.4s 循环）——只用于「激活态」元素；页面标题静态不故障（可读性优先）。
- 滚动条：深底 + 青色发光轨道块。
- 终端页：近黑底 + 霓虹绿前景、青蓝 prompt 色主题，边框霓虹。
- **图表**：series 线用霓虹色（CPU 青蓝 / 内存 霓虹绿 / 磁盘 琥珀 / 网络 洋红），`areaStyle` 同色系 12% 半透明渐变，坐标轴文字 `#7B85A3`、网格线 `rgba(33,230,255,.10)`，发光 emphasis。

### 3.5 黑暗主题（dark）—— 简洁暗色版
- 基底深暖灰 `#1D1E24`，面板 `#26272E`，hover `#30313A`，文字 `#E6E7EC`，次级 `#A2A5B0`。
- 保留雾蓝/柔粉/青绿语义色，饱和度略提亮以保证对比度（如 `--blue → #6C9FD6`）。
- **无网格/噪点/霓虹**，仅深色 + 微光；卡片为纯色（非玻璃）。
- Element Plus 全套 `--el-*` 变量切换，保证弹窗/消息/菜单正确换色。

## 四、实施方案

### 4.1 主题机制
| 文件 | 改动 |
|---|---|
| **新增** `src/theme.js` | `THEMES=[{name:'light',label:'光明'},{name:'dark',label:'黑暗'},{name:'cyber',label:'赛博朋克'}]`；`getStoredTheme()` 读 localStorage `sm2_theme`；`applyTheme(name)` → 写 `document.documentElement.dataset.theme` + localStorage + 更新响应式 `themeState`（vue reactive）；导出 `themeState` 供组件 watch；导出 `getChartTheme(name)`（图表坐标轴/系列色映射）、`getTerminalTheme(name)`（xterm 映射） |
| [main.js](file:///f:/forProgram/INDEXxiaoxia/GUI/Xx1ServerManager2/prj/frontend/src/main.js) | `createApp` 之前先 `applyTheme(getStoredTheme())`，避免启动闪白后跳主题 |

### 4.2 style.css（核心，结构调整）
1. 保留现有 `:root`（光明）不动。
2. 新增语义变量层（在 `:root` 定义默认值，dark/cyber 覆盖）：
   `--page-bg`、`--panel-bg`、`--panel-bg-2`、`--hover-bg`、`--header-bg`、`--aside-bg`、
   `--terminal-bg`、`--terminal-fg`、`--terminal-accent`、`--err-fg`、`--card-border`、
   `--stat-grad-1/2/3`（统计卡渐变终点）、`--sel-scrollbar` 等；并将现有组件层的 `rgba` 阴影改为引用 `--el-box-shadow-*`（主题内覆盖）。
3. 新增 `:root[data-theme='dark']` 覆盖块：语义变量 + `--el-*`（bg/text/border/fill/table/box-shadow/scrollbar）。
4. 新增 `:root[data-theme='cyber']` 覆盖块：同上 + 特效层（`#app` 背景改网格渐变、光斑改霓虹、噪点层、数据流动画、面板玻璃+霓虹描边发光、表格/输入/菜单荧光、故障帧动画）；关键选择器（`.card`、`header`、`aside`、`el-table`、输入聚焦等）利用已变量化属性自动生效，特效控制在约 15 条 CSS 规则内。
5. 保留 `prefers-reduced-motion` 降级（现有已有，扩展覆盖新动画）。

### 4.3 硬编码迁移清单（改 `<style>` 内的 hex → 语义变量）
| 文件 | 位置 → 替换为 |
|---|---|
| Layout.vue | `#FBFAF8→var(--aside-bg)`、`#F1EFEA→var(--hover-bg)`、`#FFFFFF→var(--header-bg)`、`rgba(255,255,255,.72)→var(--header-bg)` |
| ServerSelector.vue | 树背景 `#FBFAF8→var(--panel-bg-2)` |
| Execute.vue | 面板 `#FBFAF8→var(--panel-bg-2)`、输出块 `#23262D/#C7CBD4/#E5979D→var(--terminal-bg)/var(--terminal-fg)/var(--err-fg)`、`#C4C6CE→var(--ink-3)` |
| Upload.vue | 组色兜底 `#C4C6CE→var(--ink-3)` |
| services/Index.vue | 状态块 `#23262D/#C7CBD4→var(--terminal-bg)/var(--terminal-fg)` |
| terminal/Index.vue | 全页深色 → 终端语义变量（cyber 下自动霓虹）；xterm theme 改用 `getTerminalTheme(themeState.name)`，并 watch `themeState` 对已存在会话执行 `term.options.theme = ...` |
| dashboard/Index.vue | 渐变终止色 `#F2F6FA/#F0F6F2/#FBF2F3→var(--stat-grad-1/2/3)`；进度条高亮阈值色 `#C2757C→var(--red)`、`#5780A9→var(--blue)` 等 |
| presets/Index.vue | 空态边框 `#F0D2D5→var(--el-border-color)` |
| Unlock.vue | 光斑 rgba 与阴影 → 语义变量（浅色暗色淡光斑、cyber 霓虹光斑） |
| style.css | 滚动条 `#D5D2CA/#C4C0B8→var(--sel-scrollbar)`；光斑/卡片边框阴影 rgba → 覆盖变量 |

> groups 页的「分组颜色色板 / 用户选择色」属于用户数据，不迁移。

### 4.4 图表联动（Monitor.vue）
- `buildOption` 从 `getChartTheme(themeState.name)` 取：series 主色、坐标轴文字色、轴线色、网格线色、tooltip 背景。
- watch `themeState.name` → 对 4 个 chart 各 `setOption`（仅样式键，不动 series 数据：`axisLabel.color`、`axisLine.lineStyle.color`、`splitLine.lineStyle.color`、`textStyle`、`tooltip`），数据曲线保留。
- 首次 `initCharts` 时读取当前主题；切主题也会即时刷新样式。

### 4.5 设置页与持久化
- [settings/Index.vue](file:///f:/forProgram/INDEXxiaoxia/GUI/Xx1ServerManager2/prj/frontend/src/views/settings/Index.vue)：下拉选项加「赛博朋克（cyber）」，`@change` 时立即 `applyTheme(settings.theme)` + `save()` 落库；`onMounted` 时按当前主题回填。
- 数据库 `settings.theme` 与 localStorage 双写（启动以 localStorage 为准——可在解锁前应用；库用于同步）。

## 五、假设与决策

- 主题状态**不等同**于服务器设置中的业务项：以 localStorage 为主、DB 为备份，二者在设置页保存时同步。
- 不引入 Element Plus 官方 dark 主题样式文件（该文件是给 `html.dark` 用的），手动覆盖 `--el-*` 变量即可获得完整组件换肤且与自定义配色一致。
- 赛博朋克**特效控制清单**：网格背景、静态噪点、2 处霓虹光斑、1 条数据流动画、面板玻璃+霓虹描边发光、激活态轻微故障帧、滚动条发光 —— 其余全部走变量换色，保证流畅。
- 分组颜色为用户自定义色，不随主题变化。
- 主题切换入口保留在设置页（现成下拉位），不做顶栏快捷按钮（避免侵占顶栏布局紧凑性）。

## 六、验证步骤

1. `npm run build` 前端编译通过；`wails build` 打包成功。
2. 设置页选择三种主题，逐一验证：
   - 光明：与现有一致，无回归。
   - 黑暗：全局深色、表格/弹窗/下拉/消息均变暗、文字对比度正常。
   - 赛博朋克：暗底+网格+噪点+玻璃面板+霓虹描边发光；激活菜单/进度条/按钮发光；监控图表标尺清晰；终端霓虹配色。
3. 重启程序：主题保持上次选择（localStorage 生效，且解锁前首页已应用）。
4. 切换主题时：监控页图表坐标轴/网格/系列色即时刷新且历史曲线不丢失；已打开的 SSH 终端配色即时更新。
5. 实测 `prefers-reduced-motion`（若有）下动画停用；赛博朋克下页面滚动流畅。