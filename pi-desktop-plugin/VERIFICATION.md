# PI-Desktop 插件当前验证状态

## 外部 Deja 接入 — 2026-10-23

### 实现状态

- 已完成源码实现，位于 Deja-vu 仓库 `pi-desktop-plugin/`，与 `codex-plugin/` 并列。旧独立目录保留为历史，不再作为本轮维护或交付源。
- 基线：Deja-vu `8881e7e566c4b020251dfb455b60b2e7e4419630`；任务分支 `fix/pi-desktop-plugin-external-deja`。
- 包不包含 Deja 引擎或本机路径。宿主启动 Node stdio 桥，桥调用外部 `deja mcp`；hooks 和 `/deja` 菜单通过相同运行时配置定位同一外部程序、索引、notes、policy、源路径设置。
- 配置持久化到用户配置目录，修改后下一次调用生效，无需重新打包。程序缺失时仍可使用 `/deja` 配置；失效的已保存路径不会被隐去或悄悄回退。
- `/deja` 提供程序选择/验证、状态、共享位置、Agent 路径、源文件发现、启用切换，以及确认后的手动索引。自动索引默认关闭，开启需确认，仍受共享策略约束。
- MCP 增加必填的当前项目 `cwd`，以真实项目目录启动外部 MCP；缺失或相对目录会拒绝，防止把 note 归到插件目录。其他参数转发给外部 Deja，不重写检索与记忆逻辑。
- 手动索引没有两分钟硬限制，仍由取消/停止/释放所有权终止；查询和 hook 保留有界超时。
- 版本维持 `0.1.1` 开发基线，不代表正式发布。无需重新编译 Go 引擎；测试使用已有的 `0.23.0-pi-desktop.2` 外部程序。

### 实际检查

环境：Windows，Node v26.5.0，既有 sqlite3；测试历史、配置、index、notes 全部位于会话 scratch 的合成夹具中。没有索引用户真实历史。

| 检查 | 结果 | 范围 |
|---|---|---|
| `node --check` | 已通过 | 五个运行模块、四个测试文件、staging 脚本 |
| `DEJA_TEST_BINARY=... node --test --test-timeout=20000 test/*.test.mjs` | **57 通过，0 失败，0 跳过** | 最终 14.874 秒；外部 MCP 转发、staged manifest 真正启动桥、共享索引与双向 notes、项目 cwd 归属、路径更新、Desktop 历史重写/WAL/软删除/脱敏、hooks、压缩、菜单确认、取消及进程树清理 |
| 单轮独立源码审查 | 已完成 | 修复真实项目 cwd 缺失、手动索引时间限制、失效程序路径不可见、路径输入和错误提示；补对应回归。关于关闭实例复用的建议经宿主源码核实不适用：工厂按 Runner 创建，关闭后的对象应保持销毁 |
| 初轮测试失败 | 已修复并复测 | 两项测试断言/夹具问题；新增宿主式测试清理目录先于关闭桥造成 Windows EPERM，改为先等待子进程关闭再删除夹具，无重试删除 |
| PI 官方 devkit `check` 源码调用 | 已通过 | `ok: true`，无错误；仅提示 `agent.extension`、`mcp.server.local` 需要用户授权 |
| 会话 `PluginCheck` / `PluginPack` | **被工具限制阻止** | 拒绝工作区外的 `D:/Codes/Memories/deja-vu/pi-desktop-plugin`；当前打开项目仍是 Obsidian。遵照用户要求，不往 Obsidian 写 staging，不用 shell/普通 ZIP 绕过打包限制 |
| `.piplug` | **本轮未生成** | 旧独立目录/旧版本包不代表本轮源码，不能当作本轮安装包使用 |
| 源码 staging 与无二进制检查 | 已通过测试 | staging 仅复制运行源码和文档；manifest 使用 `node src/mcp-bridge.mjs`，无 `bin`、EXE、DLL、个人配置或 UNCONFIGURED 模板 |
| 应用内 UI / provider E2E | 未执行 | 没有自动安装插件、重启应用或调用模型；菜单通过模拟宿主 UI 测试，MCP 通过真实 stdio 子进程测试 |
| Go 全量重跑 | 不适用本轮 | 本轮没有 Go 源码变化；不把此前的全量超时或跳过项描述为已解决 |

最终完整日志：会话 scratch 中 `pi-plugin-external-tests-4.log`。安装和真实历史索引仍需明确授权/操作。

### 交付与剩余步骤

源码入口：`pi-desktop-plugin/manifest.json`，用户说明：`README.md`。

可用 `node scripts/stage.mjs <新的绝对目录>` 生成仅运行源码目录。PI-Desktop 的“Load unpacked plugin / 加载未打包插件”可选该目录进行本地验收；这不代表本轮已替用户安装。

要得到安装包：把 PI-Desktop 当前项目切换到 **Deja-vu 仓库**，再用官方 `PluginCheck` 和 `PluginPack` 处理 `pi-desktop-plugin`，或其只含运行文件的 staging 目录。本轮不通过写入 Obsidian 或手工 ZIP 规避限制。

本机先前命令发现没有找到稳定安装的 `deja`，仅有原开发测试二进制。因此插件完成不等于已安装 Deja；启用后应在 `/deja` 指定用户稳定安装的适配版本。Node 和 sqlite3 已在当前 Agent 环境找到，但 GUI 环境仍需应用内诊断确认。

### 工作区与发布边界

开始时已有无关改动：三个 marketplace 清单与 `.claude/commands/harness.md` 删除、`.agents/skills/` 未跟踪。保持原样，不纳入本次提交。

未改 PI-Desktop 核心、未写 Obsidian、未迁移/删除任何历史、未安装依赖或插件、未执行真实索引、未推送/合并/打标签/正式发布。普通开发版本不自动升级。
