# 测试报告

这里只记录一次运行的结果。阶段门槛在 [总规范](../planning/PLAN.md)，实施步骤在 [实施计划](../planning/implementation/)。测试代码在 `edge/tests/`，不把用例正文复制到这里。

四层目录与代码目录同名：

```text
docs/verification/
├── unit/           # edge/tests/unit
├── contract/       # edge/tests/contract
├── integration/    # edge/tests/integration
└── hardware/       # edge/tests/hardware
```

文件名是 `pN-短名.md`。同一阶段、同一层只保留最新一份，下次运行替换结论，不另存历史副本。某一层还没有运行时，不预建空报告。

当前记录：

- [P1 单元测试](unit/p1-local-data-chain.md)
- [P2 单元测试](unit/p2-uplink.md)
- [P2 契约测试](contract/p2-telemetry.md)
- [P3 单元测试](unit/p3-local-warnings.md)
- [P3 契约测试](contract/p3-events.md)
