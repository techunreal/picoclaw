# Third-party notices

This repository is a community fork of [sipeed/picoclaw](https://github.com/sipeed/picoclaw)
(MIT). Some fixes are adapted from the actively maintained community fork
[afjcjsbx/picoclaw](https://github.com/afjcjsbx/picoclaw) (MIT).

## Adapted changes

- `pkg/agent/agent.go`, `pkg/agent/hook_mount.go`,
  `pkg/agent/hook_mount_test.go`: fail-closed approval-hook replacement during
  config reload, adapted from afjcjsbx/picoclaw commit `63bb5573`
  ("fix(agent): keep approval hooks mounted when reload fails (#484)").
- `pkg/config/defaults.go`, `pkg/tools/shell.go`, `pkg/tools/cron.go`,
  `pkg/constants/channels.go`: default remote command execution to disabled,
  inspired by afjcjsbx/picoclaw commit `852e77e8`
  ("Default exec remote execution to disabled (#553)"). This fork extends the
  change so the local Web UI (`pico`) and CLI keep working while external chat
  channels stay blocked unless explicitly enabled.

Both projects are distributed under the MIT License. See `LICENSE`.
