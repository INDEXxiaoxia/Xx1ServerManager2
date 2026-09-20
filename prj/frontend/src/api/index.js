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
  setMasterPassword: (p) => App.SetMasterPassword(p),
  verifyMasterPassword: (p) => App.VerifyMasterPassword(p),

  // 服务器
  listServers: () => asArray(App.ListServers()),
  getServer: (id) => App.GetServer(id),
  saveServer: (s) => App.SaveServer(s),
  deleteServer: (id) => App.DeleteServer(id),
  deleteServers: (ids) => App.DeleteServers(ids),
  testConnection: (host, port, username, password) => App.TestServerConnection(host, port, username, password),
  testStoredServer: (id) => App.TestStoredServer(id),
  setServerStatus: (id, status) => App.SetServerStatus(id, status),

  // 分组
  listGroups: () => asArray(App.ListGroups()),
  saveGroup: (g) => App.SaveGroup(g),
  deleteGroup: (id) => App.DeleteGroup(id),

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
  terminalResize: (id, cols, rows) => App.TerminalResize(id, cols, rows),
  terminalClose: (id) => App.TerminalClose(id)
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