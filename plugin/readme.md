# 目录结构

```text
plugin/
  plugin.go                         # 统一接口、Manifest、Registry
  loader/
    loader.go                       # 集中导入已编译插件子包
```

主程序导入 `plugin/loader` 并调用 `loader.DefaultRegistry()`。`loader` 通过 blank import 导入插件子包，插件子包在 `init()` 中调用 `plugin.Register(...)` 完成注册。

# 统一接口

每个插件实现 `plugin.Interface`：

```go
type Interface interface {
    Manifest() plugin.Manifest
    Apply(context.Context, *plugin.Context) error
}
```

`Manifest()` 声明插件元数据和适用范围：

```go
plugin.Manifest{
    Name:        "gemini-include-thoughts",
    Label:       "Gemini include thoughts",
    Description: "Force generationConfig.thinkingConfig.includeThoughts to true.",
    Handlers:    []utils.HandlerType{utils.HandlerGemini},
    APITypes:    []utils.APIType{utils.APIGemini},
}
```

`Apply` 接收当前请求上下文并直接修改它。一个模型启用多个插件时，会按 `models.plugin` 中的名称顺序依次执行。

# 请求上下文

`plugin.Context` 包含：

- `Alias`：用户请求里的模型别名
- `Origin`：别名解析后的上游模型名
- `Handler`：当前模型解析后的后端处理器，例如 `gemini`
- `APIType`：当前 API 类型，例如 `gemini`
- `Stream`：当前请求是否为流式请求

请求体通过方法访问：

- `JSON()`：返回当前请求体的共享 `*ast.Node`。多个 JSON 插件会复用同一个 AST。
- `SetBody([]byte)`：直接替换请求体，并清空已经解析的 AST。
- `Bytes()`：返回最终请求体。如果已经解析 JSON，会在插件链末尾统一 marshal。

# 新增插件

1. 在 `plugin/<handler/api>/<plugin-name>/` 下创建插件子包
2. 实现 `plugin.Interface`
3. 在 `init()` 中调用 `plugin.Register(Plugin{})`
4. 在 `plugin/loader/loader.go` 添加 blank import

示例：

```go
package myplugin

import (
    "context"

    "github.com/nekohy/MeowCLI/plugin"
    "github.com/nekohy/MeowCLI/utils"
)

type Plugin struct{}

func init() {
    plugin.Register(Plugin{})
}

func (Plugin) Manifest() plugin.Manifest {
    return plugin.Manifest{
        Name:        "gemini-my-plugin",
        Label:       "Gemini my plugin",
        Description: "Describe the request mutation.",
        Handlers:    []utils.HandlerType{utils.HandlerGemini},
        APITypes:    []utils.APIType{utils.APIGemini},
    }
}

func (Plugin) Apply(ctx context.Context, req *plugin.Context) error {
    // 不需要修改时直接返回 nil
    return nil
}
```

然后在 `plugin/loader/loader.go` 加载：

```go
import (
    "github.com/nekohy/MeowCLI/plugin"

    _ "github.com/nekohy/MeowCLI/plugin/<handler/api>/<plugin-name>"
)
```

# 在模型上启用插件

`models.plugin` 字段保存模型启用的插件名称，使用英文逗号分隔：

```text
gemini-include-thoughts,gemini-my-plugin
```

管理端 API 约定：

- 模型列表返回每个模型的 `plugin` 字段
- overview 中每个 handler 返回当前可用的 `plugins` 列表
- 前端根据模型 handler 过滤可选插件

请求转发时，bridge 会读取当前模型解析结果中的启用插件列表，并在请求发往上游前按顺序执行
