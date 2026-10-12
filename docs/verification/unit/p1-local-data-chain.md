# P1 单元测试报告

日期：2026-10-12。结论：通过。

本报告对应 `edge/tests/unit/` 与实施计划第 3.6 节。测试不连接 Broker，不打开摄像头，不访问 GPIO。

## 环境

| 项 | 值 |
| --- | --- |
| 系统 | Windows，win32 |
| Python | 3.13.12 |
| pytest | 9.1.1 |
| PyYAML | 6.0.3 |
| 工作目录 | `edge/` |

## 命令与结果

```text
python -m pytest tests/unit -v --tb=no
```

29 项收集，29 项通过，0 项失败，耗时 0.86 秒。

```text
PYTHONPATH=src python -c "import piguard_edge; print(piguard_edge.__version__)"
```

输出 `0.0.0`。

## 验收对照

| 门槛 | 用例 | 结果 |
| --- | --- | --- |
| 故障值保持为空，不能变成 0 | `test_fault_value_stays_null_in_fixture`、`test_timeout_zero_is_stored_empty`、`test_invalid_lane_fixture_keeps_offset_empty` | 通过 |
| 静止速度 0 不被距离映射处理 | `test_ok_zero_speed_is_preserved`、`test_zero_speed_is_not_passed_through_distance_mapping`、`test_zero_speed_scene_stays_zero` | 通过 |
| 六个场景共用一个时钟，同一时刻可重复 | `test_scenario_names_match_the_command_contract`、`test_same_time_is_the_same_keyframe_after_rewind`、`test_sources_share_one_scene_and_keep_image_empty` | 通过 |
| 播放倍率只缩放关键帧间隔 | `test_playback_speed_only_scales_keyframe_time`、`test_rejected_playback_speed` | 通过 |
| 序列结束后停在最后一帧 | `test_sequence_holds_the_last_keyframe` | 通过 |
| `scenario.stop` 后恢复 `normal_drive` | `test_stop_restores_normal_drive_on_the_next_read` | 通过 |
| 距离只缩放一次 | `test_road_distance_is_divided_once_and_mapped_once` | 通过 |
| 正偏移为 right，负偏移为 left | `test_lane_departure_positive_offset_is_right`、`test_negative_offset_is_left` | 通过 |
| `sensor_failure` 只清空被点名的源 | `test_sensor_failure_empties_only_the_named_source` | 通过 |
| 加载器接受 `hardware`，mock 不读取引脚和地址 | `test_example_config_keeps_hardware_and_resolves_storage`、`test_mock_drivers_do_not_receive_hardware`、`test_missing_hardware_is_rejected`、`test_unknown_top_level_key_is_rejected` | 通过 |
| 推进 1 秒的采样次数与 fake clock 时间戳 | `test_one_second_matches_the_sample_rates` | 通过 |
| 一路失败时其他源继续更新 | `test_one_failed_source_does_not_stop_the_others` | 通过 |
| `yaw_axis` 来自配置，不来自 mock 对象上的地址 | `test_yaw_uses_configured_axis_and_sign`、`test_sampler_uses_configured_yaw_axis` | 通过 |
| mock 路径不导入 gpiozero、smbus2、Picamera2 | `test_p1_sources_do_not_import_hardware_libraries` | 通过 |
| 线程外壳调用同一个 `step` | `test_loop_calls_the_same_step` | 通过 |

1 秒采样的实测次数是距离 10、车速 10、陀螺仪 50、温度 1、摄像头 10。温度样本的单调时间是 1 秒，UTC 来自注入的 fake clock。

## 用例文件

| 文件 | 数量 | 覆盖 |
| --- | --- | --- |
| `edge/tests/unit/test_models.py` | 4 | 故障夹具、无效车道、负偏移、速度 0 |
| `edge/tests/unit/test_scale.py` | 3 | 距离一次缩放、速度不进入映射、偏航轴与符号 |
| `edge/tests/unit/test_scenarios.py` | 9 | 六个场景、回绕、播放倍率、停止恢复、传感器故障 |
| `edge/tests/unit/test_loader.py` | 6 | 示例配置、`hardware`、未知键、`local.yaml`、非 mock 模式 |
| `edge/tests/unit/test_sampler.py` | 6 | 采样周期、故障隔离、配置偏航轴、采样循环 |
| `edge/tests/unit/test_imports.py` | 1 | 硬件库导入 |

## 本报告未覆盖

契约、联调、硬件三层还没有测试，也没有对应报告。规则持续时间、MQTT、命令去重、OpenCV 和真实传感器都不在这次运行里。mock 通过不算硬件通过。
