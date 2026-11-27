import {ElMessageBox} from 'element-plus'
let queue = Promise.resolve()
window.runtime.EventsOn('server:root-request', request => {
 queue = queue.catch(()=>{}).then(async () => {
  try {
   const {value} = await ElMessageBox.prompt(`服务器「${request.server}」需要 root 权限。输入当前 SSH 用户的 sudo 密码，授权后重试操作；密码仅在内存中按「设置 → 安全」配置的时间缓存，各服务器独立。重复执行命令可能重复已完成的步骤。`, '请求 root 权限', {inputType:'password',inputPlaceholder:'留空则使用当前用户密码作为root密码',confirmButtonText:'授权并重试',cancelButtonText:'取消',closeOnClickModal:false})
   await window.go.main.App.ResolveRootRequest(request.id,value || '',true)
  } catch { await window.go.main.App.ResolveRootRequest(request.id,'',false) }
 })
})
