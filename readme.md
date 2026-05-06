# MeowCLI

> 一个注重高性能与调度的 API 转发服务

## 特性

- 开箱即用，支持 SQLite / PostgreSQL，并通过 `sqlc` 生成代码以优化查询性能
- 冷启动/重启秒恢复状态
- 跟随对话/定时同步额度，综合 5 小时与 7 天窗口的剩余额度和重置时间评分，优先选择更优凭据，而不是随机选择
- 可以单独指定某模型的调用套餐类型（如pro/free），也可指定调用顺序
- 请求失败后自动触发基于 `Retry-After` 或指数退避的临时熔断
- atomic 和 otter 缓存层，规避高延迟 SQL 操作
- 支持创建多个 Key 用于内部分发
- 前端使用 Nuxt SSG 构建，且符合MD3风格

* gemini-cli由于header未返回配额信息，只能定时同步配额/报错时自动同步,codex为实时读取header，但不会入库，会影响性能
* 由于atomic+otter的缓存机制，它的内存占用不会很小，但是性能比数据库频繁读写快多了

## 配置方式

### 环境变量

| 变量名 | 说明 | 默认值 |
| --- | --- | --- |
| `LISTEN_ADDR` | 服务监听地址 | `:3000` |
| `DATABASE_URL` | 数据库地址；为空时使用 SQLite 文件 | `` |
| `DB_TYPE` | 数据库类型，支持 `sqlite` / `postgres` | `sqlite` |
| `DEBUG` | 设为 `TRUE` 启用 pprof 调试服务，监听 `:6060` | `False` |

PostgreSQL 连接 URL 示例：

```text
postgres://meowcli:meowcli@postgres:5432/meowcli?sslmode=disable
```

## 管理面板

浏览器打开：

```text
http://127.0.0.1:3000/admin
```

首次启动时，页面会提示创建第一个管理员密钥。

### 配置模型映射

调用模型接口之前，需要先在管理台创建模型映射：

- `alias`：对外暴露的模型别名
- `origin`：真实上游模型名
- `handler`：映射的 CLI 类型

### 接口调用密钥

在管理台“密钥”页面创建一个 `user` 或 `admin` 密钥：

- `admin`：完整管理权限
- `user`：只能访问模型接口

注意：

- 日志只保存在内存中，服务重启后会清空
- 日志保留时间可以在设置页调整

## 接口支持

- `codex`：提供原生 OpenAI Responses 接口；Completion 接口与非流式 Response 由内置转换器转换
- `gemini-cli`：提供原生 Gemini 接口

## 效果图

<table>
  <tr>
    <td width="50%"><img src="image/PixPin_2026-05-06_13-49-45.png" alt="管理台首页" width="100%"></td>
    <td width="50%"><img src="image/PixPin_2026-05-06_13-50-07.png" alt="模型映射页面" width="100%"></td>
  </tr>
  <tr>
    <td width="50%"><img src="image/PixPin_2026-05-06_13-50-25.png" alt="密钥页面" width="100%"></td>
    <td width="50%"><img src="image/PixPin_2026-05-06_13-50-53.png" alt="日志页面" width="100%"></td>
  </tr>
  <tr>
    <td width="50%"><img src="image/PixPin_2026-05-06_13-51-47.png" alt="用量页面" width="100%"></td>
    <td width="50%"><img src="image/PixPin_2026-05-06_13-52-03.png" alt="额度页面" width="100%"></td>
  </tr>
  <tr>
    <td width="50%"><img src="image/PixPin_2026-05-06_13-52-11.png" alt="设置页面" width="100%"></td>
    <td width="50%"><img src="image/PixPin_2026-05-06_13-52-26.png" alt="接口调用示例一" width="100%"></td>
  </tr>
  <tr>
    <td width="50%"><img src="image/PixPin_2026-05-06_13-52-56.png" alt="接口调用示例二" width="100%"></td>
    <td width="50%"><img src="image/PixPin_2026-05-06_13-53-19.png" alt="接口调用示例三" width="100%"></td>
  </tr>
</table>

## 须知

- 我不会写前端，前端纯AI的
- Gemini 个人号未经过太多测试，没那么多号
- 各种格式转换与计费系统不是反代该做的
- 没做防封禁处理，使用者需自行承担风险

## 开发指南

### 环境要求

- Go 1.25+
- Node.js 22+
- [sqlc](https://sqlc.dev/)：仅在修改 `db/*/schema` 或 `db/*/queries` 后需要重新生成 `internal/db/*`

### 本地开发

```bash
# 生成 SQL 代码
sqlc generate

# 启动完整本地开发环境（Go 后端 + Nuxt HMR）
make frontend-dev

# 完整构建（前端 SSG + Go 二进制）
make build
```

### 常用命令

```bash
make release       # 生成发布二进制和 checksum
make clean         # 清理构建产物
```
