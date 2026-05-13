# MetaBlog CLI 使用文档

MetaBlog 只提供一个命令行二进制入口：`metablog`。所有网站构建、单篇文章构建、文章元数据维护和缓存维护都通过这个入口完成。

## 命令结构

推荐使用新的子命令结构：

```text
metablog site build
metablog site init
metablog site serve
metablog article build
metablog article init
metablog article edit
metablog article delete
metablog cache clean
```

旧版根命令参数仍然保留兼容：

```text
metablog -site -root .
metablog -input path/to/main.tex -out out
```

## 构建整个网站

```bash
metablog site build -root . -out out
```

该命令会读取站点配置、文章配置、关于页面和所有未删除的文章，生成完整静态网站。

常用参数：

| 参数 | 默认值 | 说明 |
| --- | --- | --- |
| `-root` | `.` | 项目根目录。 |
| `-out` | `out` | 静态网站输出目录。 |
| `-config` | `data/config.toml` | 网站配置文件。 |
| `-articles` | `data/articles.toml` | 文章元数据配置文件。 |
| `-force` | `false` | 强制重新编译所有文档，即使输出仍然新鲜。 |
| `-no-assets` | `false` | 跳过文档资源复制和 PDF 转 SVG。 |
| `-no-latexml-cache` | `false` | 忽略 LaTeXML 复杂块缓存。 |
| `-subset-fonts` | `false` | 根据生成的 HTML 内容对子集化字体。 |
| `-article-workers` | `0` | 并行构建文章的 worker 数，0 表示自动选择。 |
| `-latexml-workers` | `0` | 并行转换 LaTeXML 复杂块的 worker 数，0 表示自动选择。 |
| `-latexml-bin` | 空 | 指定 `latexmlc` 可执行文件路径。 |
| `-strict` | `false` | 出现解析 warning 时构建失败。 |
| `-keep-temp` | `false` | 保留 LaTeXML 临时文件。 |

示例：

```bash
metablog site build -root . -out out -article-workers 4
metablog site build -root . -force -no-latexml-cache
metablog site build -root . -subset-fonts
```

## 预览已生成的网站

```bash
metablog site serve -out out
```

该命令会把已生成的静态网站目录作为本地 HTTP 服务暴露出来，适合构建后快速预览。命令会阻塞运行，直到按 `Ctrl+C` 停止。

参数：

| 参数 | 默认值 | 说明 |
| --- | --- | --- |
| `-out` | `out` | 要预览的静态网站输出目录。 |
| `-host` | `127.0.0.1` | 监听地址。 |
| `-port` | `0` | 监听端口；`0` 表示由系统随机选择一个空闲端口。 |

示例：

```bash
metablog site serve -out out
metablog site serve -out out --host 0.0.0.0 --port 8080
```

启动后会输出实际访问地址，例如：

```text
Serving /path/to/site/out
URL: http://127.0.0.1:51324/
Press Ctrl+C to stop.
```

### 文件监听和热重编译（Watch 模式）

启用 `-watch` 后，`site serve` 在启动 HTTP 服务器之外，还会持续监听已注册文章的源目录、关于页面和站点配置文件的变化。当监测到文件修改时，自动对变更部分执行增量编译（启用 LaTeXML 缓存），无需手动重新构建：

```bash
metablog site serve -out out -watch -root .
```

Watch 模式参数：

| 参数 | 默认值 | 说明 |
| --- | --- | --- |
| `-watch` | `false` | 启用文件监听和热重编译。 |
| `-root` | `.` | 项目根目录；watch 模式需要此参数来定位文章源文件和配置。 |
| `-config` | `data/config.toml` | 站点配置文件（用于 watch 模式）。 |
| `-articles` | `data/articles.toml` | 文章元数据配置文件（用于 watch 模式）。 |
| `-latexml-bin` | 空 | `latexmlc` 可执行文件路径（用于 watch 模式）。 |
| `-article-workers` | `0` | watch 模式下的并行文章编译数；0=自动。 |
| `-latexml-workers` | `0` | watch 模式下的并行 LaTeXML 转换数；0=自动。 |
| `-no-assets` | `false` | watch 模式下跳过资源复制和 PDF 转 SVG。 |

监听范围：

1. **文章源目录**：`data/articles.toml` 中每篇文章的 `folder` 目录，源文件修改时仅重编译该文章的 HTML。
2. **关于页面**：`data/about_page/` 目录，修改时重编译关于页面。
3. **站点配置**：`data/config.toml`，修改时更新站点资源，重新生成所有索引页面，并强制重编译关于页面和所有文章页，确保 header、logo、icon 等站点级信息一致。
4. **文章元数据**：`data/articles.toml`，修改时重新加载文章列表、重新生成所有索引页面，并检查所有已注册文章页；缺失或源文件更新过的文章会被增量编译。

检测机制为每秒轮询源目录中最晚文件修改时间，并使用 300ms 去抖动避免编辑过程中频繁编译。

示例：

```bash
# 构建后启动 watch 模式预览
metablog site build -root . -out out && metablog site serve -out out -watch -root .

# 跳过资源处理以加快重编译速度
metablog site serve -out out -watch -root . -no-assets
```

启动后日志会显示监听的文章数量和重编译摘要：

```text
Serving /path/to/site/out
URL: http://127.0.0.1:51324/
Watch: monitoring 3 article(s) and about page for changes
Press Ctrl+C to stop.
Watch: rebuilt My Article (0 warning(s), source=articles/my-article/)
```

## 初始化网站目录

```bash
metablog site init -root my-blog -title "My Blog"
```

该命令用于从空白目录初始化一个 MetaBlog 网站。初始化过程会创建必要目录、写入默认配置文件、写入默认 about 页面、写入默认 logo/icon、写入字体 CSS，并下载项目默认字体文件。

`site init` 按“只有一个 `metablog` 二进制文件”的使用场景设计：初始化时不会读取当前源码仓库中的 `asset/`、`web/static/`、`docs/` 或模板文件。默认 logo/icon、`fonts.css`、配置模板和 about 页面模板都已经编译进二进制；字体文件从网络下载到新网站目录的 `web/static/fonts/`。

会创建或检查的目录：

| 目录 | 说明 |
| --- | --- |
| `articles/` | 文章目录。 |
| `asset/figs/` | 站点 logo 和 icon 等资源目录。 |
| `data/about_page/` | 关于页面 LaTeX 文档目录。 |
| `web/static/fonts/` | Web 字体目录。 |

会初始化的文件：

| 文件 | 说明 |
| --- | --- |
| `data/config.toml` | 网站标题、logo、icon 和分页配置。 |
| `data/articles.toml` | 空文章列表配置。 |
| `data/about_page/main.tex` | 默认关于页面。 |
| `asset/figs/circle_example.svg` | 默认站点 logo 和 icon 示例图。 |
| `web/static/fonts.css` | 默认字体声明。 |
| `.gitignore` | 忽略 `out/`、`.metablog-cache/`、`.gocache/` 和 `.gomodcache/`。 |

已有文件默认不会被覆盖；再次执行 `site init` 是安全的，命令只会补齐缺失项。

参数：

| 参数 | 默认值 | 说明 |
| --- | --- | --- |
| `-root` | `.` | 要初始化的网站根目录。 |
| `-title` | `MetaBlog` | 写入 `data/config.toml` 的站点标题。 |
| `-latexml-bin` | 空 | 环境检测时使用的 `latexmlc` 路径。 |
| `-skip-fonts` | `false` | 跳过字体下载，只创建目录和配置文件。 |
| `-skip-env-check` | `false` | 跳过 Python、LaTeXML 和 PDF 转换器检测。 |

初始化末尾会执行环境检测，但不会自动安装或配置 Python/LaTeXML。检测失败时，WARN 后面会附带安装建议：

| 检测项 | 说明 |
| --- | --- |
| `latexmlc` | 检测 LaTeXML 是否可解析路径和版本。 |
| `pyftsubset` | 检测 fonttools 的字体子集化命令。 |
| `python packages` | 检测 Python 是否可以导入 `fontTools` 和 `brotli`。 |
| `PDF converter` | 检测 `pdftocairo`、`mutool` 或 `inkscape` 是否存在。 |

环境检测结果只输出 `OK` 或 `WARN`，不会修改本机环境。

常见安装方式：

| 环境项 | Windows | macOS | Debian/Ubuntu |
| --- | --- | --- | --- |
| LaTeXML | 安装 Strawberry Perl，然后运行 `cpan LaTeXML` 或 `cpanm LaTeXML`，并确保 `latexmlc.bat` 在 PATH 中。 | `brew install latexml` | `sudo apt install latexml` |
| Python | 安装 python.org 的 Python 3 或 Conda，并确保 `python` 在 PATH 中。 | `brew install python` | `sudo apt install python3 python3-pip` |
| fonttools/brotli | `python -m pip install fonttools brotli` | `python3 -m pip install fonttools brotli` | `python3 -m pip install fonttools brotli` |
| PDF 转换器 | 安装 Poppler for Windows 并把 `bin` 目录加入 PATH；也可安装 MuPDF 或 Inkscape。 | `brew install poppler`，或安装 MuPDF/Inkscape。 | `sudo apt install poppler-utils`，或安装 `mupdf-tools`/`inkscape`。 |

## 构建单篇 LaTeX 文档

```bash
metablog article build -input articles/example/main.tex -out out/example
```

该命令只编译一个 LaTeX 主文件，适合调试解析器、调试单篇文章渲染，或生成独立 HTML 页面。

常用参数：

| 参数 | 默认值 | 说明 |
| --- | --- | --- |
| `-input` | `sample_latex/DACE-with_supplementary.tex` | 主 LaTeX 文件。 |
| `-out` | `out` | 输出目录，生成的页面为 `index.html`。 |
| `-root` | `.` | 项目根目录，同时用于定位 `.metablog-cache/`。 |
| `-dump-ast` | `false` | 输出调试 AST 到 `out/debug/ast.json`。 |
| `-no-assets` | `false` | 跳过资源处理。 |
| `-no-latexml-cache` | `false` | 忽略 LaTeXML 复杂块缓存。 |
| `-latexml-workers` | `0` | 并行转换 LaTeXML 复杂块的 worker 数。 |
| `-latexml-bin` | 空 | 指定 `latexmlc` 可执行文件路径。 |
| `-strict` | `false` | 出现解析 warning 时构建失败。 |
| `-keep-temp` | `false` | 保留 LaTeXML 临时文件。 |

示例：

```bash
metablog article build -input data/about_page/main.tex -out out/about-debug -dump-ast
metablog article build -input articles/my-paper/main.tex -out out/my-paper -no-latexml-cache
```

## 维护文章配置

文章配置维护命令会读写 `data/articles.toml`，并在 `articles/` 下创建或更新对应文章目录。

```bash
metablog article init -root .
metablog article edit -root .
metablog article delete -root .
```

可用参数：

| 参数 | 默认值 | 说明 |
| --- | --- | --- |
| `-root` | `.` | 项目根目录。 |
| `-articles` | `data/articles.toml` | 文章元数据配置文件。 |

`article init` 和 `article edit` 在输入 `description` 时，会一直读取多行内容，直到遇到连续两个换行才结束。

## 清理缓存

```bash
metablog cache clean -root .
```

该命令会删除项目根目录下的 `.metablog-cache/`。如果缓存目录不存在，命令直接成功返回。

当前缓存主要包含 LaTeXML 复杂块缓存：

```text
.metablog-cache/latexml/
```

也可以手动删除 `.metablog-cache/` 清理缓存；CLI 会在删除前校验目标目录确实是项目根目录内的 `.metablog-cache`，避免误删其他路径。

## 兼容入口

为了兼容旧脚本，下面两个旧入口仍然可用：

```bash
metablog -site -root . -out out
metablog -input articles/example/main.tex -out out/example
```

后续新增功能优先放入子命令结构中，旧入口只作为兼容层保留。
