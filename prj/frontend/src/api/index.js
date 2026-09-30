// Wails Go 后端 API 封装
const App = window.go.main.App

// 确保后端列表返回值为数组：Go 侧可能返回 null/undefined（nil 切片），
// 此处统一兜底为空数组，避免前端对 null 调用 .length/.forEach 等导致白屏
const asArray = (p) => p.then((v) => (Array.isArray(v) ? v : []))

export const api = {
  // 应用/认证
  getAppInfo: () => App.GetAppInfo(),
  isMasterPasswordSet: () => App.IsMasterPasswordSet(),
  isUnlocked: () => App.IsUnlocked(),
  isLocked: () => App.IsLocked(),
  lock: () => App.Lock(),
  unlockWithoutPassword: () => App.UnlockWithoutPassword(),
  setMasterPassword: (p) => App.SetMasterPassword(p),
  verifyMasterPassword: (p) => App.VerifyMasterPassword(p),
  changeMasterPassword: (oldPassword, newPassword) => App.ChangeMasterPassword(oldPassword, newPassword),

  // 服务器
  listServers: () => asArray(App.ListServers()),
  getServer: (id) => App.GetServer(id),
  saveServer: (s) => App.SaveServer(s),
  deleteServer: (id) => App.DeleteServer(id),
  deleteServers: (ids) => App.DeleteServers(ids),
  testConnection: (host, port, username, password) => App.TestServerConnection(host, port, username, password),
  testStoredServer: (id) => App.TestStoredServer(id),

  // 分组
  listGroups: () => asArray(App.ListGroups()),
  saveGroup: (g) => App.SaveGroup(g),
  deleteGroup: (id) => App.DeleteGroup(id),
  // 批量分配：勾选集合即该分组的成员集合（groupID 传空串表示移出分组）
  assignGroupServers: (groupID, serverIDs) => App.AssignGroupServers(groupID, serverIDs),

  // 预设
  listPresetCommands: () => asArray(App.ListPresetCommands()),
  savePresetCommand: (c) => App.SavePresetCommand(c),
  deletePresetCommand: (id) => App.DeletePresetCommand(id),
  listPresetPaths: () => asArray(App.ListPresetPaths()),
  savePresetPath: (p) => App.SavePresetPath(p),
  deletePresetPath: (id) => App.DeletePresetPath(id),

  // 设置
  getSettings: () => App.GetSettings(),
  saveSettings: (s) => App.SaveSettings(s),
  getConfigDirInfo: () => App.GetConfigDirInfo(),
  openConfigDir: () => App.OpenConfigDir(),

  // 终端 AI（ChatGPT 兼容 OpenAI 协议）
  // 检测类调用都要"先订阅 ai:* 事件、再发起请求"——sessionId 由前端生成，
  // 否则极快失败时（例如 Key 无效）首个事件会在订阅之前发出，界面会一直转圈。
  getAISettings: () => App.GetAISettings(),
  saveAISettings: (s) => App.SaveAISettings(s),
  testAIConnection: () => App.TestAIConnection(),
  aiChat: (req) => App.AIChat(req),
  aiExecOne: (termId, sessionId, command) => App.AIExecOne(termId, sessionId, command),
  aiStartTakeover: (req) => App.AIStartTakeover(req),
  aiStopTakeover: (sessionId) => App.AIStopTakeover(sessionId),
  aiTakeoverControl: (sessionId, action) => App.AITakeoverControl(sessionId, action),
  aiReset: (termId) => App.AIReset(termId),
  aiTerminalSnapshot: (termId, mode, lines, selection) => App.AITerminalSnapshot(termId, mode, lines, selection),

  // 批量上传/执行
  startBatchUpload: (serverIds, localPaths, remoteDir) => App.StartBatchUpload(serverIds, localPaths, remoteDir),
  startBatchCommand: (serverIds, command, timeout) => App.StartBatchCommand(serverIds, command, timeout),
  getCommandHistory: (limit) => asArray(App.GetCommandHistory(limit)),

  // 文件管理
  listRemoteDir: (serverId, path) => asArray(App.ListRemoteDir(serverId, path)),
  makeRemoteDir: (serverId, path) => App.MakeRemoteDir(serverId, path),
  renameRemote: (serverId, oldPath, newPath) => App.RenameRemote(serverId, oldPath, newPath),
  deleteRemote: (serverId, path, isDir) => App.DeleteRemote(serverId, path, isDir),
  chmodRemote: (serverId, path, mode) => App.ChmodRemote(serverId, path, mode),
  uploadToServer: (serverId, localPath, remotePath) => App.UploadToServer(serverId, localPath, remotePath),
  downloadFromServer: (serverId, remotePath, localPath) => App.DownloadFromServer(serverId, remotePath, localPath),
  selectLocalFile: () => App.SelectLocalFile(),
  selectLocalSavePath: (defaultName) => App.SelectLocalSavePath(defaultName),

  // 文件管理 · 目录栏 / 搜索 / 目录大小 / 在线编辑
  resolveRemotePath: (serverId, path) => App.ResolveRemotePath(serverId, path),
  validateRemoteDir: (serverId, path) => App.ValidateRemoteDir(serverId, path),
  getRemoteDirSizes: (serverId, paths) => asArray(App.GetRemoteDirSizes(serverId, paths)),
  searchRemoteDir: (serverId, root, keyword, recursive) => App.SearchRemoteDir(serverId, root, keyword, recursive),
  detectRemoteEncoding: (serverId, path) => App.DetectRemoteEncoding(serverId, path),
  readRemoteTextFile: (serverId, path, encoding) => App.ReadRemoteTextFile(serverId, path, encoding),
  writeRemoteTextFile: (serverId, path, req) => App.WriteRemoteTextFile(serverId, path, req),

  // 实时监控
  getServerStats: (serverId) => App.GetServerStats(serverId),
  startMonitor: (serverId) => App.StartMonitor(serverId),
  stopMonitor: (serverId) => App.StopMonitor(serverId),
  stopAllMonitors: () => App.StopAllMonitors(),
  getMonitorHistory: (serverId, limit) => asArray(App.GetMonitorHistory(serverId, limit)),
  getTopProcesses: (serverId, limit) => asArray(App.GetTopProcesses(serverId, limit)),

  // 服务管理
  listServices: (serverId) => asArray(App.ListServices(serverId)),
  getServicesWithEnabled: (serverId) => asArray(App.GetServicesWithEnabled(serverId)),
  serviceAction: (serverId, name, action) => App.ServiceAction(serverId, name, action),
  getServiceStatus: (serverId, name) => App.GetServiceStatus(serverId, name),

  // SSH 终端
  terminalOpen: (serverId, cols, rows) => App.TerminalOpen(serverId, cols, rows),
  terminalWrite: (id, data) => App.TerminalWrite(id, data),
  terminalWriteMany: (ids, data) => App.TerminalWriteMany(ids, data),
  terminalResize: (id, cols, rows) => App.TerminalResize(id, cols, rows),
  terminalClose: (id) => App.TerminalClose(id),
  // 在指定服务器上执行一次性命令（不占用交互式终端会话）
  runRemoteCommand: (serverId, command, timeout) => App.RunRemoteCommand(serverId, command, timeout),

  // 运维工具 · 磁盘空间检测
  // 检测是异步的：StartDiskAnalysis 返回 taskId，进度与报告通过
  // ops:disk:progress|report|done:<taskId> 事件推送。
  // taskId 由调用方先生成并「先订阅、再发起」，避免极快失败时漏掉首个事件。
  listServerDisks: (serverId) => asArray(App.ListServerDisks(serverId)),
  startDiskAnalysis: (serverId, mount, useSudo, taskId) => App.StartDiskAnalysis(serverId, mount, useSudo, taskId),
  cancelDiskAnalysis: (taskId) => App.CancelDiskAnalysis(taskId),
  // 返回 {items, partial, note}：partial 表示 du 被超时中断、items 只是统计到的部分
  drillDiskDir: (serverId, mount, path, useSudo) => App.DrillDiskDir(serverId, mount, path, useSudo)
}

// Wails 事件封装
export const events = {
  on(name, cb) {
    return window.runtime.EventsOn(name, cb)
  },
  off(name, cb) {
    window.runtime.EventsOff(name, cb)
  },
  emit(name, data) {
    window.runtime.EventsEmit(name, data)
  }
}