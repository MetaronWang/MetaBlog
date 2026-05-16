# MetaBlog Example

这是一个完整的 MetaBlog 示例站点，站点名为 `MetaBlog Example`。

从仓库根目录构建示例站点：

```powershell
go run ./cmd/metablog site build -root example -out example/out
```

启动带文件监听和自动刷新的本地预览：

```powershell
go run ./cmd/metablog site serve -root example -out example/out -watch
```

示例站点刻意使用 SVG 文章主图和轻量 `fonts.css`，因此不需要下载大体积字体文件也能构建。如果需要完整默认字体，可以运行 `metablog site init -root example`；已有文件不会被覆盖，命令只会补齐缺失资源。
