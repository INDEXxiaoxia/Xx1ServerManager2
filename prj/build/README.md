# Build Directory

The build directory is used to house all the build files and assets for your application. 

The structure is:

* bin - Output directory
* darwin - macOS specific files
* windows - Windows specific files

## Mac

The `darwin` directory holds files specific to Mac builds.
These may be customised and used as part of the build. To return these files to the default state, simply delete them
and
build with `wails build`.

The directory contains the following files:

- `Info.plist` - the main plist file used for Mac builds. It is used when building using `wails build`.
- `Info.dev.plist` - same as the main plist file but used when building using `wails dev`.

## Windows

The `windows` directory contains the manifest and rc files used when building with `wails build`.
These may be customised for your application. To return these files to the default state, simply delete them and
build with `wails build`.

- `icon.ico` - The icon used for the application. This is used when building using `wails build`. If you wish to
  use a different icon, simply replace this file with your own. If it is missing, a new `icon.ico` file
  will be created using the `appicon.png` file in the build directory.
- `installer/*` - The files used to create the Windows installer. These are used when building using `wails build`.
- `info.json` - Application details used for Windows builds. The data here will be used by the Windows installer,
  as well as the application itself (right click the exe -> properties -> details)
- `wails.exe.manifest` - The main application manifest file.

## 图标（本项目自定义流程）

Windows 图标由 `windows/icon.ico` 决定，macOS 与安装包图标由 `appicon.png` 决定。
`appicon.png` 是 Wails 默认图标的备份；本项目已替换为自己的图标。

重新生成（换图标时）：

```bash
# 1. 把新的方形大图放到 prj/build/icon-source.png（建议 >= 512x512，作为源图不入库）
# 2. 生成 windows/icon.ico（256/128/64/48/32/24/16）与 appicon.png（1024x1024）
node prj/build/make-icon.js prj/build/icon-source.png prj/build/windows/icon.ico prj/build/appicon.png --appicon
# 3. 校验 exe 内嵌的图标是否已替换（比对每个尺寸的 SHA1）
node prj/build/verify-icon.js prj/build/bin/prj.exe prj/build/windows/icon.ico
```

说明：

- `make-icon.js` 纯 Node 实现（PNG 解码/缩放/编码 + ICO 组装），不依赖外部图形库；
  会裁掉四周全透明边距（可用 `--no-trim` 关闭），并按 Windows 规范对 >=64 的尺寸用
  PNG 条目、<64 的尺寸用 BMP 条目。
- 注意：`icon.ico` 目录项声明的尺寸必须与实际图片尺寸一致，否则 Windows 会拉伸导致模糊
  （有些在线转换工具生成的"目录声明 256×256、实际只有 32×32"的 ico 就是常见坑）；
  `verify-icon.js` 会一并检查声明尺寸与实际尺寸是否一致。
- 更换后若资源管理器仍显示旧图标，是 Windows 图标缓存所致：重启资源管理器或清理缩略图缓存。