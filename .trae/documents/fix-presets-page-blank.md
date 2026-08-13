# 修复计划：预设管理页面渲染空白

## 1. 摘要

预设管理页（`prj/frontend/src/views/presets/Index.vue`）从初版（el-tabs 结构）到现版（el-radio-group 结构）多次出现**内容区空白**，最新版本已恶化到**整个内容区全白**（左侧菜单、顶部标题正常）。由于该应用无可用的 F12 控制台，且静态审查未发现报错点，本计划通过「**错误可见化 + 结构降级为已验证可渲染的同款写法 + dev 模式复核**」三步闭环，彻底定位并修复。

## 2. 现状分析（探索结论）

### 2.1 现象证据（用户反馈）
| 版本 | 结构 | 现象 |
|---|---|---|
| v1 | el-tabs + `<el-icon><Plus/></el-icon>` | tab 头显示、pane 内容空白 |
| v2 | el-tabs + `:icon="'Plus'"` | 同上，用户仍看不到按钮 |
| v3 | el-radio-group + v-if 表格 | **整个内容区全白** |

- 空白时**左侧菜单、顶部标题正常** → 是 presets 路由组件**渲染失败**，非全局崩溃
- **其他页面（服务器列表、批量执行等）全部正常** → 问题只存在于预设页组件

### 2.2 代码审查结论
- [presets/Index.vue](file:///f:/forProgram/INDEXxiaoxia/GUI/Xx1ServerManager2/prj/frontend/src/views/presets/Index.vue)（现版）：模板、script、样式静态检查无语法错误；`vite build` 已通过。
- [radio-group 组件源码](file:///f:/forProgram/INDEXxiaoxia/GUI/Xx1ServerManager2/prj/frontend/node_modules/element-plus/es/components/radio/src/radio-group.vue_vue_type_script_setup_true_lang.mjs)：`value` prop 在 element-plus **2.14.4** 受支持（`use-radio.mjs` 的 `actualValue = props.value ?? props.label`），不会抛错。
- [main.js](file:///f:/forProgram/INDEXxiaoxia/GUI/Xx1ServerManager2/prj/frontend/src/main.js)：全量注册 Element Plus 组件与图标，无 errorHandler。
- Go 侧绑定方法 `ListPresetCommands` / `ListPresetPaths` 等已在 [App.js](file:///f:/forProgram/INDEXxiaoxia/GUI/Xx1ServerManager2/prj/frontend/wailsjs/go/main/App.js) 中确认存在。

### 2.3 根因判断
**主假设（高概率）**：presets 组件在**渲染期**抛出一个未知运行时错误（Vue 将错误冒泡，该组件子树不挂载，故内容区空白；无 errorHandler 时只在 console 报错，用户看不到）。直接静态定位不可行，因此必须**先让错误在应用界面可见**。

**次假设**：用户运行了旧构建/旧资源缓存。通过 v3 新结构（分段按钮样式）是否出现来区分——用户已确认看到"整页空白"，疑似 v3 已生效，故主假设优先。

## 3. 修改方案

### 3.1 全局错误可见化 — `prj/frontend/src/main.js`
- 注册 `app.config.errorHandler(err, instance, info)` 与 `window.onerror` / `window.onunhandledrejection`：
  - 将错误信息 `formatErrorMessage(err, info)` 后，通过 `document.title` + 页面顶部注入一个固定样式**红色错误条**（不依赖 console、不依赖 Element 组件，保证任何页面出错都能看到文字）。
  - 同时 `console.error` 输出（dev 模式可看）。
- 目的：下次任何页面（尤其预设页）若再出错，用户可直接把错误条内容反馈，一步定位。

### 3.2 预设页结构降级 — `prj/frontend/src/views/presets/Index.vue`
**原则**：只使用与「服务器列表页」完全相同的、已被验证可正常渲染的组件与结构（`el-button` / `el-table` / `el-table-column` / `el-tag` / `el-dialog` / `el-form` / `el-input` / `el-select` / `el-option`），并仿照其「工具行 + 表格」布局：

1. **移除** `el-radio-group` / `el-radio-button`（v3 新增、唯一未在其他页面使用过的组件）。
2. **切换方式**：顶部工具行左侧两个 `el-button`（`预设命令` / `预设路径`），用 `:type` + `v-show` 切换，右侧一个「新建命令 / 新建路径」按钮（跟随当前 tab）。
3. **内容**：两个区块用 `v-show`（而非 v-if）切换，每区块 = 卡片工具行 + `el-table`（空数据时 el-table 自带「暂无数据」提示）。
4. **保留**：新增/编辑弹窗（`el-dialog`）+ 分类 `el-select` + 表单逻辑与 `api` 调用不变（`openAdd`/`openEdit`/`save`/`remove`/`load`）。
5. **组件内兜底**：在 `<script setup>` 加入 `onErrorCaptured(err, instance, info)`，返回 `false` 并把错误渲染进页内一个 `ref('')` 显示为红色文字，形成双重诊断。

### 3.3 交互微调（不属本次 bug 但已在计划内并行的界面问题）
- 不做其它功能改动，专注修复空白。

## 4. 假设与决策
- **不**继续在 el-tabs 与 el-radio 上尝试，因为两套写法均出现渲染异常，证明决策点应转向「错误可见 + 结构降级」。
- 保留所有 Go 后端方法不变（前端绑定已确认存在且签名匹配）。
- 修复过程中**不为诊断而临时改坏数据流**：`load()` 的异常保持 `try/catch`，失败时页面显示错误条而非白屏。

## 5. 验证步骤
1. `cd prj/frontend && npm run build` — 通过，无编译错误。
2. `cd prj && wails build` — 打包成功，产出 `prj/build/bin/prj.exe`。
3. 用户在桌面环境运行新 exe，进入「预设管理」：
   - ✅ 顶部显示「预设命令 | 预设路径」两个切换按钮 + 右侧「新建命令」按钮
   - ✅ 切换后有对应表格（空数据显示「暂无数据」）
   - ✅ 新增/编辑弹窗正常
4. 若仍空白：页面顶部会出现**红色错误条**（global errorHandler 注入）或组件内 onErrorCaptured 错误文本，用户把文字内容反馈后按错误精确二次修复。
5. 让用户体验一次 `wails dev`（开发模式自带 devtools，可 F12 看 console），进一步复核。

## 6. 涉及文件
- `prj/frontend/src/main.js` — 加 errorHandler + window.onerror 错误条
- `prj/frontend/src/views/presets/Index.vue` — 结构降级重写 + onErrorCaptured