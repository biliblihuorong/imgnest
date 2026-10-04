@AGENTS.md

## Claude Code 补充

- 项目 Skill 位于 `.claude/skills/`，会被自动发现；无需再安装。
- 动手写代码前先按 `AGENTS.md` 的表格加载对应 Skill；涉及上传/存储/EXIF 时必须加载 `imgnest-image-pipeline`，涉及 `/api/v1` 时必须加载 `lsky-api-compat`。
- 规范或版本有出入时，以 `docs/spec.md` 和 `docs/versions.md` 为准，并在回复里指出冲突。
