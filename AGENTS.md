# Repository Guidelines
## General

### Basic Rules

- 不要改动未指定的业务代码
- 你只需且只能阅读并按指令和规则编辑代码文件

### Coding

- 根据现有代码，保持代码风格
- 可复用的代码抽象成方法
- 类内可复用的方法抽象成类内的工具方法
- 跨类可复用的方法抽象成utils/下的全局工具方法
- 每个方法级函数或对象都要添加注释，采用Javadoc标准格式
- 新增或修改函数、类等对象时必须要对应更新注释
- 在记录日志时，除了专有名词可以用英文缩写，其他内容全部用中文


## Post Coding

- 每次如果有新的接口添加 或 有接口经过任何改动，都需要对应更新 `backend/test/api.postman.json`
- 保持该文件原本的规则和格式

## Multi-Agent Workflow

### Goal

- 采用 5 个固定 Agent 的协作模式，确保从需求到评审的链路可追踪、可交接、可复用。

### Prompt Location

- 5 个 Agent 的通用提示词模板统一存放在 `prompts/agents/` 目录。
- 每个新 thread 开始时，必须先注入对应 Agent 的角色提示词，再下发具体任务。
- 角色提示词负责“身份与职责对齐”；本 `AGENTS.md` 负责“项目共性规则约束”。

### Agent Startup Protocol

- 每个 Agent 在确认自身身份后，必须先主动阅读 `prompts/agents/` 下对应模板，再开始任何执行动作。
- 模板映射关系如下：
  - Agent 1 -> `prompts/agents/agent1-requirement-interface.md`
  - Agent 2 -> `prompts/agents/agent2-change-plan.md`
  - Agent 3 -> `prompts/agents/agent3-implementation.md`
  - Agent 4 -> `prompts/agents/agent4-testing-qa.md`
  - Agent 5 -> `prompts/agents/agent5-review-architecture.md`
- 若角色与模板不匹配，或模板缺失，当前 Agent 不得开始工作，需先修复模板映射。
- Agent 开工前需完成模板适配自检（最小检查集）：
  - 是否明确角色目标与边界（做什么/不做什么）
  - 是否明确输入、输出格式与 DoD
  - 是否明确上下游依赖与门禁（特别是 Agent 2 -> Agent 3 的 `LATEST.md` 约束）
  - 是否与本仓库通用规则冲突（注释、日志语言、接口变更后文档更新等）
- 若模板不满足上述检查项，Agent 需先补齐模板再执行任务。

### Agent Roles
- 所有的内容都区分前后端，需要放在前/后端目录下。例如：在写后端时，`devdocs/`

- Agent 1：需求与接口设计 Agent  
  负责需求澄清、边界定义、接口设计、数据模型草案、验收标准（AC）。
- Agent 2：代码改动规划 Agent  
  负责输出文件/类/方法级改动计划、伪代码、数据库变更、实施顺序、回滚点。
- Agent 3：开发实现 Agent  
  负责严格按 Agent 2 的计划实现代码与测试，并记录与计划差异。
- Agent 4：测试与质量 Agent  
  负责基于 AC 编写单元测试并自动执行测试与回归，输出缺陷分级与放行结论。
- Agent 5：评审与架构守卫 Agent  
  负责最终审查正确性、安全性、性能与可维护性，并给出准入意见。

- Agent 1 的产出必须是 Markdown 文件，并落盘到 `devdocs/` 目录。
- Agent 2 的产出必须是 Markdown 文件，并落盘到 `.ai-workspace/` 目录。
- Agent 2 每次产出后必须同步更新 `.ai-workspace/LATEST.md`，用于声明最新可执行计划文件。
- Agent 3 只能基于 `.ai-workspace/` 下由 Agent 2 最新产出的“新需求改动计划”执行代码编辑。
- 若 `.ai-workspace/LATEST.md` 不存在，或其指向的计划文件不存在，Agent 3 不得开始实现。
- Agent 4 只能改动 `backend/test/` 或 `frontend/test/` 下的测试文件，不得改动其他业务代码。
- Agent 4 编写的单元测试必须符合最佳实践（可读、可复现、稳定、覆盖正常/异常/边界路径）。
- Agent 4 必须自动执行测试命令并记录结果。
- Agent 4 目标测试覆盖率不低于 80%。

### Process Gates

- 未完成 Agent 1 输出前，不允许进入 Agent 2。
- 未完成 Agent 2 输出前，不允许进入 Agent 3。
- Agent 3 不得跳过 Agent 2 计划直接实现；如需偏离，必须记录“差异原因与影响”。
- 只有 Agent 3 可以修改业务代码。
- Agent 4 在提交测试结论前必须完成“编写单元测试 + 自动执行”。
- Agent 4 的测试结论与 Agent 5 的评审结论，作为是否合并的最终依据。

### Deliverables

- 每个 Agent 的输出必须包含：可交接物、完成定义（DoD）、风险与待确认项（如有）。
- 接口有新增或修改时，除代码变更外，仍必须同步更新 `backend/test/api.postman.json`。
