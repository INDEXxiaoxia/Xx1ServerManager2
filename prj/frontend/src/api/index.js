// Wails Go 后端 API 封装
const App = window.go.main.App

export const api = {
  // 应用/认证
  getAppInfo: () => App.GetAppInfo(),
  isMasterPasswordSet: () => App.IsMasterPasswordSet(),
  isUnlocked: () => App.IsUnlocked(),
  setMasterPassword: (p) => App.SetMasterPassword(p),
  verifyMasterPassword: (p) => App.VerifyMasterPassword(p),

  // 服务器
  listServers: () => App.ListServers(),
  getServer: (id) => App.GetServer(id),
  saveServer: (s) => App.SaveServer(s),
  deleteServer: (id) => App.DeleteServer(id),
  deleteServers: (ids) => App.DeleteServers(ids),
  testConnection: (host, port, username, password) => App.TestServerConnection(host, port, username, password),
  testStoredServer: (id) => App.TestStoredServer(id),
  setServerStatus: (id, status) => App.SetServerStatus(id, status),

  // 分组
  listGroups: () => App.ListGroups(),
  saveGroup: (g) => App.SaveGroup(g),
  deleteGroup: (id) => App.DeleteGroup(id),

  // 预设
  listPresetCommands: () => App.ListPresetCommands(),
  savePresetCommand: (c) => App.SavePresetCommand(c),
  deletePresetCommand: (id) => App.DeletePresetCommand(id),
  listPresetPaths: () => App.ListPresetPaths(),
  savePresetPath: (p) => App.SavePresetPath(p),
  deletePresetPath: (id) => App.DeletePresetPath(id),

  // 设置
  getSettings: () => App.GetSettings(),
  saveSettings: (s) => App.SaveSettings(s),

  // 批量上传/执行
  startBatchUpload: (serverIds, localPaths, remoteDir) => App.StartBatchUpload(serverIds, localPaths, remoteDir),
  startBatchCommand: (serverIds, command, timeout) => App.StartBatchCommand(serverIds, command, timeout),
  getCommandHistory: (limit) => App.GetCommandHistory(limit),

  // 文件管理
  listRemoteDir: (serverId, path) => App.ListRemoteDir(serverId, path),
  makeRemoteDir: (serverId, path) => App.MakeRemoteDir(serverId, path),
  renameRemote: (serverId, oldPath, newPath) => App.RenameRemote(serverId, oldPath, newPath),
  deleteRemote: (serverId, path, isDir) => App.DeleteRemote(serverId, path, isDir),
  chmodRemote: (serverId, path, mode) => App.ChmodRemote(serverId, path, mode),
  uploadToServer: (serverId, localPath, remotePath) => App.UploadToServer(serverId, localPath, remotePath),
  downloadFromServer: (serverId, remotePath, localPath) => App.DownloadFromServer(serverId, remotePath, localPath),
  selectLocalFile: () => App.SelectLocalFile(),
  selectLocalSavePath: (defaultName) => App.SelectLocalSavePath(defaultName),

  // 实时监控
  getServerStats: (serverId) => App.GetServerStats(serverId),
  startMonitor: (serverId) => App.StartMonitor(serverId),
  stopMonitor: (serverId) => App.StopMonitor(serverId),
  stopAllMonitors: () => App.StopAllMonitors(),
  getMonitorHistory: (serverId, limit) => App.GetMonitorHistory(serverId, limit),
  getTopProcesses: (serverId, limit) => App.GetTopProcesses(serverId, limit),

  // 服务管理
  listServices: (serverId) => App.ListServices(serverId),
  getServicesWithEnabled: (serverId) => App.GetServicesWithEnabled(serverId),
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