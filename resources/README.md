# 泽 · 观天镜 macOS App Icon

网络地图使用 `world-land.json`：由 [Natural Earth 1:110m land](https://github.com/nvkelso/natural-earth-vector/blob/master/geojson/ne_110m_land.geojson) 转换，仅保留陆地外轮廓并将坐标四舍五入到两位小数。数据属于[公共领域](https://www.naturalearthdata.com/about/terms-of-use/)。应用首次打开分析地图时生成一次缓存纹理，不下载在线瓦片；地图节点和连线仅表达估算位置与连接去向。

这套资源基于你选中的“蓝紫星环”图标整理，适用于 macOS App。

## 11 个可直接使用的图标资源
1. icon_16x16.png — 16×16
2. icon_16x16@2x.png — 32×32
3. icon_32x32.png — 32×32
4. icon_32x32@2x.png — 64×64
5. icon_128x128.png — 128×128
6. icon_128x128@2x.png — 256×256
7. icon_256x256.png — 256×256
8. icon_256x256@2x.png — 512×512
9. icon_512x512.png — 512×512
10. icon_512x512@2x.png — 1024×1024
11. Guantianjing.icns — macOS App 图标文件

另外附带：
- AppIcon_1024.png — 1024×1024 主图，方便后续二次制作。

## Xcode 使用
推荐直接把 `Guantianjing.iconset` 中对应尺寸拖入 Assets.xcassets 的 AppIcon；
也可以在传统 macOS 工程中使用 `Guantianjing.icns`。

## 说明
当前源图尺寸约 332×324，本包已统一裁切为正方形并重采样到各标准尺寸。
如果后续要上架 Mac App Store，建议再生成一张原生 1024×1024 无损母版，以获得最佳大尺寸显示效果。
