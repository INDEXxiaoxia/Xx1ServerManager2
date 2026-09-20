# Xx1ServerManager2 UI 样式重构方案

## 一、Summary（方案摘要）

在**不改动任何业务逻辑、不改变数据流、不新增文案**的前提下，对 Wails 前端（Vue3 + Element Plus）进行整体视觉重构：

- 建立低饱和粉蓝黄（雾蓝 / 柔粉 / 奶油黄）+ 暖纸底色的设计体系，通过 CSS 变量统一覆盖 Element Plus 主题；
- 重构全局样式（style.css）、主框架（Layout.vue / Unlock.vue）；
- 逐页打磨 10 个视图与 2 个公共组件（卡片、表格、按钮、图表、分割控件、终端外壳等）；
- 加入克制的"呼吸感"动效（熵入场、悬浮、脉冲状态点、页面过渡），保证丝滑不喧宾夺主；
- 终端内容区保持深色（符合终端可读性习惯，见"假设"2）。

验证方式：`npm run build` 保证编译通过；如环境允许，`wails dev` 冒烟测试逐页功能。

## 二、Current State（现状分析）

| 维度 | 现状 |
|---|---|
| 技术栈 | Vue3 + Element Plus + Pinia + Vue Router；ECharts（监控页）；xterm（终端页）；Wails2 后端 Go |
| 全局样式 | [style.css](file:///f:/forProgram/INDEXxiaoxia/GUI/Xx1ServerManager2/prj/frontend/src/style.css)：仅工具类 `.card/.mb-12/.flex-between/.text-muted/.mono`，8px 圆角卡、灰白底 `#f5f7fa`、无动效 |
| 主题接入 | [main.js](file:///f:/forProgram/INDEXxiaoxia/GUI/Xx1ServerManager2/prj/frontend/src/main.js) 依次引入 `element-plus/dist/index.css` 后再引入 `style.css`（覆盖顺序正确，**无需改 main.js**） |
| 主框架 | [Layout.vue](file:///f:/forProgram/INDEXxiaoxia/GUI/Xx1ServerManager2/prj/frontend/src/views/Layout.vue)：紫蓝渐变 logo 与激活态（`#667eea→#764ba2`，典型的"AI味"渐变，需去除）；白色侧栏与顶栏 |
| 解锁页 | [Unlock.vue](file:///f:/forProgram/INDEXxiaoxia/GUI/Xx1ServerManager2/prj/frontend/src/views/Unlock.vue)：满屏紫渐变 |
| 页面模式 | 全部 10 个视图统一 `page-container + .card` 容器；表格全用 `stripe` Element 默认蓝主题 |
| 图表 | [Monitor.vue](file:///f:/forProgram/INDEXxiaoxia/GUI/Xx1ServerManager2/prj/frontend/src/views/servers/Monitor.vue)：eCharts 硬编码色 `#409eff/#67c23a/#e6a23c/#764ba2` |
| 终端 | [terminal/Index.vue](file:///f:/forProgram/INDEXxiaoxia/GUI/Xx1ServerManager2/prj/frontend/src/views/terminal/Index.vue) 深色 chrome；Execute/Services 内的命令输出块为深色 `#1e1e1e` 底 |
| 分组色板 | groups 页 8 色为高饱和默认色，需替换为低饱和色板 |
| Theme 设置 | [store/index.js](file:///f:/forProgram/INDEXxiaoxia/GUI/Xx1ServerManager2/prj/frontend/src/store/index.js) 中 `theme` 字段仅保存、代码中无任何消费方（**现状即无效设置，不在本次范围内激活**） |

## 三、设计理念（Design Tenet）

一句话理念：**"纸上烟云"——暖纸为底、雾蓝柔粉奶油黄为三原色，留白如呼吸，动效如微风。**

用诗人（字体节奏/留白/字母间距）、剑客（层级果断、焦点清晰、不堆砌）、哲学家（克制用色、语义一致）、艺术家（曲面、层次、粒子般的光斑）四种视角统一设计，**明确规避**：紫蓝渐变、玻璃拟态堆砌、发光辉光、emoji、装饰性文案。

### 3.1 色板（低饱和度，全部校验过对比度）

```css
/* 底色 */
--paper: #F5F4F1;        /* 页面底色·暖纸 */
--paper-deep: #ECE9E3;   /* 表头/次级底 */
--card: #FFFFFF;
--line: #E7E4DE;         /* 分割线 */

/* 文字（墨灰系，不用纯黑） */
--ink: #46484F;          /* 主文字，白底对比度 >12:1 */
--ink-2: #6E7078;        /* 次级文字，>7:1 */
--ink-3: #8F919B;        /* 占位/弱化，>4:1 */

/* 三原色 + 语义色（低饱和） */
--blue: #5780A9;         /* 主色·雾蓝，白字对比约4.2:1（AA 大型文本达标）；深色着于浅底亦清晰 */
--blue-deep: #44688F;
--blue-soft: #E0E9F1;    /* 浅蓝底（配 --ink-2 文字） */
--pink: #C58A98;         /* 柔粉（装饰/强调，不承载正文） */
--pink-soft: #F8EEF1;
--yellow: #C79E4F;       /* 奶油黄（warning 语义） */
--yellow-soft: #F9F2E2;
--green: #67947A;        /* success 语义 */
--green-soft: #EAF3EE;
--red: #C2757C;          /* danger 语义 */
--red-soft: #F9EDEE;
--violet: #9B7FB8;       /* 辅助点缀 */
```

- 规则：任何 pastel 底色（*`-soft`）必配 `--ink-2` 或更深文字；文字一律不用饱和色。`-soft` 底与文字组合灰度均在 4.5:1 以上。
- 状态语义映射：在线=green、异常/错误=red、等待/连接中=yellow、其他=gray；全站一致。

### 3.2 Element Plus 变量覆盖（写入 style.css，作用于所有 EP 组件）

在 `:root` 中覆盖（含 light-3/5/7/8/9 与 dark-2 全套，保证 hover/plain/disabled 等派生色一致）：

```css
--el-color-primary: #5780A9;             /* 及 light-3..9 / dark-2 */
--el-color-success: #67947A;
--el-color-warning: #C79E4F;
--el-color-danger: #C2757C;  --el-color-error: #C2757C;
--el-color-info: #8F919B;
--el-border-radius-base: 10px;
--el-text-color-primary: #46484F; --el-text-color-regular: #5B5D66;
--el-text-color-secondary: #8F919B; --el-text-color-placeholder: #A9ABB4;
--el-border-color: #E7E4DE; --el-border-color-light: #EBE9E3; --el-border-color-lighter: #EFEDE8;
--el-bg-color: #fff; --el-bg-color-page: #F5F4F1; --el-bg-color-overlay: #fff;
--el-fill-color: #F1EFEA; --el-fill-color-lighter: #F5F3EF; --el-fill-color-extra-light: #FAF9F6;
--el-box-shadow: 0 8px 24px rgba(70,72,79,.06), 0 2px 6px rgba(70,72,79,.04);
--el-box-shadow-light: 0 4px 14px rgba(70,72,79,.07);
--el-box-shadow-lighter: 0 2px 10px rgba(70,72,79,.05);
--el-table-* 系列（表头底 paper-deep、行高、无竖边框、hover 行 #F7F5F1）
```

## 四、Proposed Changes（逐文件改动）

### A. 全局基石

**1. [style.css](file:///f:/forProgram/INDEXxiaoxia/GUI/Xx1ServerManager2/prj/frontend/src/style.css) — 全量重写样板**
- 写入 3.1 色板变量 + 3.2 的 EP 变量覆盖（放在文件顶部）。
- `html/body`：底色 `--paper`；`#app` 上叠加两个极淡径向呼吸光斑（fixed 定位、`filter: blur(90px)`、蓝/粉、透明度 ≤ 0.35，z-index 0，营造"空气感"但不抢内容）。
- 动画帧：
  - `@keyframes rise`（入场：`opacity:0; translateY(10px)` → 到位，`.3s cubic-bezier(.16,1,.3,1) both`）
  - `@keyframes breathe`（呼吸：`opacity .55↔1; scale 1↔1.03`，4.5s ease-in-out infinite；用于在线状态点/logo 圆点）
  - `@keyframes float`（悬浮：`translateY 0↔-4px`，6s；用于解锁卡）
- 工具类重构：`.card`（半径 16px、分层阴影、padding 18px 20px、`transition`）——**不给 `.card` 加 hover 位移**（避免包裹大表格时整卡浮动）；`.text-muted` → `--ink-3`；新增 `.text-2`（`--ink-2`）；`.mb-12/.mt-12/.flex-between/.mono` 语义保留。
- 滚动条：6px、暖灰 thumb、hover 加深；全局 `::selection` 淡蓝底；`*:focus-visible` 出 2px 淡蓝环。
- EP 组件全局打磨（普通规则，非 scoped）：
  - `.el-button`：半径 10px、primary 带柔和投影、hover `translateY(-1px)` 微抬、`.25s` 过渡；
  - `.el-input__wrapper`：半径 10px、focus 时外发光环 `box-shadow: 0 0 0 3px rgba(87,128,169,.12)`；
  - `.el-tag`：圆角 999px（胶囊）、默认底 `--fill-color`；
  - `.el-dialog`：半径 18px、大散阴影、`--el-dialog-border-radius` 一并设置；
  - `.el-message`：半径 12px、柔和阴影；
  - `.el-table`（经由 `--el-table-*` 变量 + 少量规则）：表头 section 无边框、半径 12px、hover 行浅暖色；保持 `stripe` 的细分模式改为淡 `--fill-color-extra-light`（条纹仍可用但极淡）。
- **不改动**：`.page-container` 结构语义；所有现有 id/class 引用均保留（避免破坏视图逻辑）。

### B. 框架与外壳

**2. [Layout.vue](file:///f:/forProgram/INDEXxiaoxia/GUI/Xx1ServerManager2/prj/frontend/src/views/Layout.vue)**
- `.aside`：背景 `#FBFAF8`，右缘 1px `--line`；宽度 220px 不变。
- `.logo`：去掉紫色渐变。左侧放 **三点落徽**（12px 圆点 ×3：雾蓝/柔粉/奶油黄，间距 6px，首点带 `breathe` 动画）；右侧 `ServerManager2`，700、letter-spacing .6px、色 `--ink`。徽标用 `span` 背景色实现，无需新增图片资源。
- 菜单 `:deep(.el-menu-item)`：间距 6px 12px、半径 12px、色 `--ink-2`；hover 底 `#F1EFEA`；**active**：底 `--blue-soft`、字 `--blue-deep`、600，伪元素左侧 3px 圆角竖条（雾蓝），过渡 `.25s`。
- `.header`：`@supports (backdrop-filter: blur(8px))` 时应答 `rgba(255,255,255,.72)+blur(12px)`，否则纯白；`.header-title` 前置 3px 渐变竖条（蓝→粉），15px/600/`--ink`；锁定按钮 hover 底 `--pink-soft`。
- `el-main` 内容区：挂 **页面过渡**（`transition name="page" mode="out-in"`，`v-slot` 解构 + `:key="route.path"`；`page-enter-from{opacity:0;translateY(10px)}`，`.3s ease`）——路由结构不变，安全。
- `term` 以外的所有页面均经此过渡（terminal 页自身无碍）。

**3. [Unlock.vue](file:///f:/forProgram/INDEXxiaoxia/GUI/Xx1ServerManager2/prj/frontend/src/views/Unlock.vue)**
- 背景：`--paper` 底 + 左右两个大 blur 圆形 wash（雾蓝/柔粉，透明度 0.4）＋ 中部奶油黄极小光斑。
- 卡片：半径 20px、双层柔和阴影、`rise` 入场 + `float` 呼吸悬浮；brand 字 22px、letter-spacing 1px，下配三点落徽（复用 B2 徽标样式，scoped 内定义）。
- 输入框半径 12px；按钮 primary、半径 12px、全宽、hover 微抬。desc 文案**保持原样**。

### C. 逐页打磨（10 个视图）

**4. [dashboard/Index.vue](file:///f:/forProgram/INDEXxiaoxia/GUI/Xx1ServerManager2/prj/frontend/src/views/dashboard/Index.vue)**
- `.stat-card`：改为带 pastel 底的卡片（总数=蓝 wash / 在线=绿 wash / 异常=红 wash），半径 18px、`rise` 入场、左上角小呼吸点（随状态着色）；数字 34px/700、字母间距 1px，色用对应深色（`--ink` 打底 + 状态色数字）。
- `.server-card`：hover `translateY(-3px)` + 阴影加深（`.3s`）；在线状态在名称前加呼吸绿点 `span.status-dot`（模板加一个条件 class，不改 JS 判定逻辑）；CPU/内存/磁盘进度条 `:color` 内联值改为雾蓝/青绿/奶油黄三色（模板内字面量替换）。
- 入场 stagger：`.server-card { animation: rise .45s ease both }` + `nth-child(n)` 递增 delay（1–12 档，最多 240ms）。

**5. [servers/List.vue](file:///f:/forProgram/INDEXxiaoxia/GUI/Xx1ServerManager2/prj/frontend/src/views/servers/List.vue)**
- 结构零改动；按钮、搜索框、状态 tag、分组 tag 全部由全局主题接管；操作列文本按钮 hover 色随主题；`more-btn` 色改 `--ink-3`。

**6. [servers/Monitor.vue](file:///f:/forProgram/INDEXxiaoxia/GUI/Xx1ServerManager2/prj/frontend/src/views/servers/Monitor.vue)**
- `buildOption` 线色替换为 `#5780A9 / #67947A / #C79E4F / #C58A98`（CPU/内存/磁盘/网络）。
- `areaStyle.opacity .1` 改为 `echarts.graphic.LinearGradient` 由上至下的同色渐变（透明度 .18→0），提升"呼吸"质感；`grid` 微调间距；tooltip 由主题接管。
- 标题 h4 色 `--ink-2`；`.chart` 高度 260px 不变；选择器不变。

**7. [commands/Execute.vue](file:///f:/forProgram/INDEXxiaoxia/GUI/Xx1ServerManager2/prj/frontend/src/views/commands/Execute.vue)**
- 双栏布局、sticky、间距全保留。`.output-card`：底 `#FBFAF8`、半径 14px、1px `--line` 边框。
- `.output-text`（命令输出深块，**保持深色以保证终端可读**）：底 `#23262D`、字 `#C7CBD4`（比原 `#1e1e1e/#d4d4d4` 稍柔）、半径 10px；`.err` 色 `#E5979D`。
- `.card-title`：16px/600 + 3px 渐变竖条前缀。
- `server-tag` 分组色为数据驱动（DB 存色），**不改逻辑**；回退色 `#dcdfe6` → `--line`。

**8. [files/Upload.vue](file:///f:/forProgram/INDEXxiaoxia/GUI/Xx1ServerManager2/prj/frontend/src/views/files/Upload.vue)**
- 与 Execute 相同的双栏样式置换（`.layout-container/.left-panel/.selector-card/.right-panel/.card-title` 同参数）；`el-progress` 状态色由主题接管。

**9. [files/Manager.vue](file:///f:/forProgram/INDEXxiaoxia/GUI/Xx1ServerManager2/prj/frontend/src/views/files/Manager.vue)**
- `.file-name:hover` 色随 primary（auto 接主题即可，删除写死的 `#409eff`）；Folder/Document 图标色 `--ink-3`；面包屑 hover 主题接管；3 个 380px 对话框样式交给全局 dialog。

**10. [groups/Index.vue](file:///f:/forProgram/INDEXxiaoxia/GUI/Xx1ServerManager2/prj/frontend/src/views/groups/Index.vue)**
- 色板替换为低饱和：`['#5780A9','#C58A98','#C79E4F','#67947A','#8F919B','#9B7FB8','#6FAE9F','#D08770']`（原高饱和 8 色删除）。
- `.color-picker .color-dot.active`：选中态由"边框+scale"改为"白色戒指环"（`box-shadow: 0 0 0 3px #fff, 0 0 0 5px var(--ink-2)`），去掉 `transform: scale`（更稳、不清新感消失）；`.color-dot` 尺寸保留。

**11. [presets/Index.vue](file:///f:/forProgram/INDEXxiaoxia/GUI/Xx1ServerManager2/prj/frontend/src/views/presets/Index.vue)**
- 伪 Tab 按钮改为 **分段控件（segmented control）**：`.tab-buttons` 底 `--fill-color`、胶囊 999px、内边距 3px；`.tab-button`（新增区分 class）active 为白底 `--card` + 柔影，过渡 `.25s`。`v-show` 切换逻辑不动。
- `.page-error` 兜底样式微调为低饱和红（底 `--red-soft`、字 `--red`、边框 1px 淡红），保留等宽字体与换行属性。

**12. [services/Index.vue](file:///f:/forProgram/INDEXxiaoxia/GUI/Xx1ServerManager2/prj/frontend/src/views/services/Index.vue)**
- `.status-text` 深色块参数与 Execute `.output-text` 对齐（`#23262D/#C7CBD4`、半径 10px）；表格与 tag 交全局主题。

**13. [settings/Index.vue](file:///f:/forProgram/INDEXxiaoxia/GUI/Xx1ServerManager2/prj/frontend/src/views/settings/Index.vue)**
- 无 scoped 样式；仅入口加一个极简 `.card-title` 式标题行样式（16px/600 + 竖条，纯视觉）。主题下拉 `light/dark` **保持原样不动**（见假设 3）。

**14. [terminal/Index.vue](file:///f:/forProgram/INDEXxiaoxia/GUI/Xx1ServerManager2/prj/frontend/src/views/terminal/Index.vue)**
- 保持深色外壳主题；微调：toolbar 底 `#23262D`、分隔线深灰；tab active 时标题白字 + 底部 2px 雾蓝指示条；关闭 hover 色 `--red`；"新建终端"按钮用 primary。xterm 实例 theme 参数**保持不变**（后端会话不受影响）。

### D. 公共组件

**15. [ServerSelector.vue](file:///f:/forProgram/INDEXxiaoxia/GUI/Xx1ServerManager2/prj/frontend/src/components/ServerSelector.vue)**
- 树容器：底 `#FBFAF8`、1px `--line` 边框、半径 12px、内边距 8px；`.tree-node` hover 底浅暖色（半径 8px）；`.node-icon` 色 `--ink-3`；选中统计 tag 用 success。`filterText/search` 逻辑不动。

**16. [ServerFormDialog.vue](file:///f:/forProgram/INDEXxiaoxia/GUI/Xx1ServerManager2/prj/frontend/src/components/ServerFormDialog.vue)**
- 无 scoped 改动；radius/阴影/输入框全交全局主题。JS 逻辑零改动。

### E. 明确不改动
- [main.js](file:///f:/forProgram/INDEXxiaoxia/GUI/Xx1ServerManager2/prj/frontend/src/main.js)、router、store、api（纯 JS/数据层）；
- 后端 Go 全部文件（无 UI 职责）；
- `wails.json`、`vite.config.js`、`.gitignore`；
- 不新增图片、字体、第三方库；不新增任何句子/文案（所有现有关键文案原样保留）。

## 五、Assumptions & Decisions（假设与决策）

1. **仅浅色方案**：按需求 7"低饱和度浅色"执行单套浅色主题；`settings` 里的 `theme` 下拉将其保持"保存但不生效"的现状（代码中本无消费方），不在本次范围激活深色模式——如需深色另立任务。
2. **终端内容区保持深色**：`xterm` 实例、命令输出块、服务状态原文块继续使用深底浅字——终端可读性与习惯使然（需求 2"易于辨认"优先），但色调用更柔和的炭灰 `#23262D` 而非死黑。
3. **零逻辑改动红线**：所有 JavaScript 业务逻辑、路由、数据流、事件订阅一律不动；仅改 `<style>` 与模板中的 class/内联色值字面量。模板改动仅限"新增装饰性 span/class"，不改任何数据绑定与判定。
4. **`.card` 本身不做 hover 位移**：仅明确可交互的元素（server-card、按钮、菜单、tab）带位移级动效，避免整页卡片哄抬的"廉价动效感"。
5. **不引入新文案**：设计通过色、形、动效表达（哲学/艺术性），不通过添加标语、诗句实现。

## 六、Verification（验证步骤）

1. `cd prj/frontend && npm run build`——确保 Vue 编译零错误（语法、模板、样式均通过）。
2. 如环境可用，`wails dev` 全量冒烟：启动后逐页检查——解锁页、总览（统计卡/服务器卡/进度条）、服务器列表（增删改/筛选/批量删除）、监控（4 图渲染、进程表）、批量执行（双栏、输出深块）、批量上传（进度）、文件管理（面包屑/双击/对话框）、分组（色板选中态）、预设（分段控件切换）、服务（状态原文）、设置、SSH 终端（新建/关闭 Tab）。
3. 对比度抽查：抽样确认 pastel 底配深字、`-soft` 底上文字 ≥4.5:1（用色板变量即达标）；禁用动画偏好（`prefers-reduced-motion: reduce`）时降级为无位移动效（在 style.css 增加一条 `@media (prefers-reduced-motion: reduce)` 兜底，保持视觉清晰）。
4. `git status` 确认只改动约定文件，无意外文件（如 dist/、bin/ 未入库）。

## 七、反思与自查（执行前检查）

- 去 AI 味：**移除**紫蓝 `#667eea→#764ba2` 渐变（Logo/解锁页/菜单激活）；**不用**玻璃拟态过量（header 玻璃仅在 `@supports` 且降级纯白）；**不用**辉光/emoji/装饰文案。
- 不呆板：色带为"暖纸 + 三点落徽 + 呼吸点"这些克制的艺术细节，而非堆色块。
- 不落俗套：分段控件、白色戒指选中环、错落 stagger 入场、渐变曲线面积图——均是克制且可执行的做法。
- 灵动/呼吸：所有动效时长 ≤ .45s、缓动 `cubic-bezier(.16,1,.3,1)`；呼吸动画仅用于状态点与 logo 点，不扰操作。
- 可辨认优先：任何配色变更均对照色板对比度规则复核后再落地。