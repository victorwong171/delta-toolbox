# ncmdump

## 简介

高吞吐、极低内存占用的网易云音乐 NCM 格式流式转换与元数据处理工具，基于 Go 语言实现。

> [!NOTE]
> 本项目基于开源项目 [yoki123/ncmdump](https://github.com/yoki123/ncmdump) 进行深度演进和架构重构。核心音频解密逻辑参考了 [anonymous5l/ncmdump](https://github.com/anonymous5l/ncmdump)。

### 🚀 演进特性与架构升级

在原有项目的基础上，完成了以下现代化升级：

1. **高性能流式顺序单趟扫描 (Single-Pass Sequential Parser)**：重构了传统的多趟 Seek 回退与 AES 重复解密逻辑，采用单次顺序流扫描机制，解密音频流在读取时 on-the-fly (即时) 解密写盘，内存分配陡降 **99.96%** (35MB 降至 80KB)，完全消除了大规模转换时的 GC 负载。
2. **Bounds Check Elimination (BCE) 越界消除优化**：通过对加密数组切片执行一次性边界断言，使 Go 编译器消除了密集解密循环内部的数组越界检查分支指令，解密速度提升 **58%**。
3. **接口解耦高内聚架构**：引入了 `NCMParser`、`MetadataManager`、`FileProcessor` 和 `ConversionPipeline` 接口，完成了 CLI 壳与业务内核的深度解耦。
4. **彻底修复 Windows 锁文件与跨平台硬编码**：将源文件读取与流式拷贝封装于自闭合作用域，彻底解决了 Windows 下删除源文件时的文件占用冲突（Sharing Violation）。消除了所有绝对物理路径硬编码，实现完美的可移植性。
5. **升级 V2 依赖与去除 Vendor**：去除了旧版大体积 `vendor` 目录，并将 FLAC 容器标签解析依赖全量平滑迁移至官方修复了文件头比对 Bug 的新版 `v2` 系列库。
6. **云原生 Docker 支持**：提供标准的多阶段构建 `Dockerfile`，生产镜像体积仅为 **~10MB**，并支持非特权安全运行与卷挂载。
7. **配置文件驱动 & 可插拔后处理器**：支持通过 `conf.yaml` 配置默认源/目标路径和后处理器插件链，无需命令行参数即可一键运行。
8. **并发 Worker 工作池**：基于 goroutine + channel 实现可配置并发数的工作池模型，大批量转换时充分利用多核。

---

## 特性

- 转换 NCM 加密文件为标准 MP3 / FLAC 音频
- 为音频文件补充 Tag 信息，包含标题、歌手、专辑、封面等
- 保留 163 key 使播放器能识别转换后的文件
- 支持配置文件驱动的批量转换模式
- 内置后处理插件：删除源文件、同名文件体积对比择优保留

---

## 📂 项目结构

```text
ncm-dumper/
├── cmd/ncmdump/              # CLI 应用入口
│   ├── main.go               # 主入口，配置加载与 DI 引导
│   ├── app.go                # CLI App 组装、并发工作池调度
│   └── conf/                 # 配置文件目录
│       ├── conf.yaml         # 默认配置文件（源/目标路径、后处理器列表）
│       └── config.go         # 配置结构体定义
├── parser/                   # NCM 解析核心
│   ├── parser.go             # SequentialNCMParser 单趟解析 + DecryptReader 流式解密
│   └── structs.go            # Meta / Album / Artist 元数据结构体
├── pipeline/                 # 转换管道
│   └── pipeline.go           # ConversionPipeline 编排解密→标签注入→后处理
├── processor/                # 后处理插件
│   ├── processor.go          # FileProcessor 接口定义
│   └── plugins.go            # DeleteSourceProcessor / SizeComparisonProcessor
├── tag/                      # 音频标签注入
│   ├── tag.go                # MetadataManager 接口 & 注入编排
│   ├── tagger.go             # Tagger 接口 & 格式分发
│   ├── mp3.go                # MP3 (ID3v2) 标签实现
│   ├── flac.go               # FLAC (VorbisComment) 标签实现
│   └── tag_test.go           # 标签单元测试
├── Dockerfile                # 多阶段 Docker 构建
├── build.sh                  # 跨平台交叉编译构建脚本
├── go.mod                    # Go 1.24 模块定义
└── go.sum
```

---

## 如何使用？

### 方式一：下载预编译二进制

从 [Releases](https://github.com/victorwang171/ncmdump/releases) 下载对应平台的可执行程序。

支持的平台：

| 操作系统 | 架构 |
|---------|------|
| Windows | amd64 / 386 |
| macOS   | amd64 |
| Linux   | amd64 / arm64 |

#### 拖拽方式

将 `.ncm` 文件或包含 `.ncm` 文件的文件夹拖拽到可执行程序上，等待转换完成。

#### 命令行方式

```bash
ncmdump [flags] [files.../dirs...]
```

**可用参数：**

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `--output` | 源文件所在目录 | 输出文件夹路径 |
| `--tag` | `true` | 是否使用 NCM 元信息为音频文件补充 Tag |
| `--workers` | `4` | 并发工作线程数 |

> [!TIP]
> 参数需要放到输入文件/文件夹之前。

```bash
# 示例：指定输出目录和并发数
ncmdump --output=D:\music_dump --workers=8 D:\music D:\music\song.ncm
```

### 方式二：配置文件驱动

程序启动时会自动搜索 `conf.yaml` 配置文件（按优先级）：
1. 当前工作目录下的 `conf/conf.yaml`
2. 当前工作目录下的 `conf.yaml`
3. 开发目录 `cmd/ncmdump/conf/conf.yaml`
4. 可执行文件所在目录
5. 系统用户配置目录（Linux: `~/.config/ncmdump/`，Windows: `%APPDATA%/ncmdump/`）

**`conf.yaml` 示例：**

```yaml
path:
    source:
        - /path/to/ncm/source/directory/
    target: /path/to/output/directory/
processors:
    - size_comparison   # 同名文件只保留体积更大的版本
    - delete_source     # 转换成功后删除源 .ncm 文件
```

配置完成后，直接运行 `ncmdump`（无需传入任何参数）即可自动批量转换。

### 方式三：作为 Go 库集成

安装：

```shell
go get -u github.com/victorwang171/ncmdump
```

#### 基础解析 — 流式单趟解密

```go
import "github.com/victorwang171/ncmdump/parser"

ncmParser := &parser.SequentialNCMParser{}
parsed, err := ncmParser.Parse(fp) // fp 为 io.Reader
if err != nil {
    log.Fatal(err)
}

// 读取元数据
meta := parsed.Metadata()     // *parser.Meta
cover := parsed.Cover()       // []byte
format := parsed.AudioFormat() // "mp3" 或 "flac"

// 流式读取解密音频数据写入目标文件
io.Copy(outFile, parsed.DecryptedStream())
```

#### 完整管道 — 解密 + 标签注入 + 后处理

```go
import (
    "github.com/victorwang171/ncmdump/parser"
    "github.com/victorwang171/ncmdump/pipeline"
    "github.com/victorwang171/ncmdump/processor"
    "github.com/victorwang171/ncmdump/tag"
)

// 组装转换管道
ncmParser := &parser.SequentialNCMParser{}
metaManager := tag.NewMetadataManager()
processors := []processor.FileProcessor{
    &processor.SizeComparisonProcessor{},
    &processor.DeleteSourceProcessor{},
}
pipe := pipeline.NewConversionPipeline(ncmParser, metaManager, processors)

// 执行单文件转换
err := pipe.Convert("/path/to/song.ncm", "/path/to/output/", true)
```

---

## 🐳 Docker 使用

使用多阶段构建，生产镜像仅 ~10MB：

```bash
# 构建镜像
docker build -t ncmdump .

# 运行转换（挂载源文件和目标目录）
docker run --rm \
    -v /host/ncm-files:/data/source \
    -v /host/output:/data/target \
    ncmdump /data/source
```

容器默认以非特权用户 `ncmuser` 运行，符合安全最佳实践。

---

## 🔌 后处理插件系统

通过 `conf.yaml` 中的 `processors` 字段或代码注入，激活转换后处理插件：

| 插件名 | 说明 |
|--------|------|
| `size_comparison` | 输出目录存在同名音频文件时，仅保留体积更大的版本 |
| `delete_source` | 转换成功且校验目标文件非空后，删除源 `.ncm` 文件 |

插件通过实现 `processor.FileProcessor` 接口进行扩展：

```go
type FileProcessor interface {
    Process(src, dst string) error
}
```

---

## 格式分析

NCM 实际上不是音频格式是容器格式，封装了对应格式的 Meta 以及封面等信息，主要的格式如下：

![ncm.png](./asserts/ncm.png)

需要解开原格式信息的关键就是拿到 AES 的 KEY，好在每个 NCM 的加密的 KEY 都是一致的（出于性能考虑）。拿到 AES 的 KEY 以后，就可以根据格式解开对应的资源。

---

## 已知问题

新版的云音乐客户端已不在 NCM 文件中嵌入图片以及 Meta 等信息。当封面数据为空时，程序会自动尝试从 `Meta.Album.CoverUrl` 远程下载封面。如果元数据也为空，标签注入将被静默跳过。如果您需要完整的 Meta 信息，建议不要使用最新版的云音乐客户端。

---

## 相关链接

- http://www.bewindoweb.com/228.html
- https://github.com/anonymous5l/ncmdump
- https://github.com/go-flac/go-flac
- https://github.com/mingcheng/ncmdump

`- eof -`
