---
generated_from_state_version: 11
---

# 验证

## 当前结果

- 结果: **已归档**
- 验证情况: **已完成检查，验证结果已确认**
- 目标周期: 1
- 迭代: 2
- 验证器尝试次数: 1
- 完成时间: 2026-10-10T09:14:09.498Z
- 摘要: 当前iteration2/attempt1候选通过全部八项静态验收；原authority缺陷已修复，未发现新的阻断语义缺陷。仅执行只读检查，未修改文件、状态或提交，未运行后置测试及外部环境。

## 验收

| 编号 | 结果 | 来源 | 验收项 | 原因 |
| --- | --- | --- | --- | --- |
| A1 | passed | brief.md | A1：默认认证与环境覆盖正确；平台/设备身份、凭据配对/冲突、TTL/origin缺失或非法在外部初始化前拒绝；显式匿名HTTP仅loopback，无默认密码或凭据日志/提交。 | 默认开启认证、密码为空且必须填写；YAML类型、非空环境覆盖、身份冲突、密码配对、seed凭据及TTL均前置校验。internal/config/config.go:96、121、237；internal/auth/config.go:20、79。server/app.go:31在建目录、数据库与MQTT连接前执行校验。origin.go:25检查原authority，已拒绝http://[localhost]:5173、http://::1:8080及非法端口。关闭HTTP认证仅允许规定loopback地址；错误不回显凭据。 |
| A2 | passed | brief.md | A2：health/合法预检之外业务统一认证授权；平台可操作原业务，设备仅上传绑定设备帧，跨设备/API/WS/图片拒绝；401/403无副作用，不回退匿名或另一凭据。 | handler/router.go:37、47将health公开，并使全部既有业务、图片与WS路由经过来源和认证授权。handler/auth.go:80、85仅允许设备POST绑定身份的frames；拒绝在业务调用前终止，使用40101/40301及no-store。auth/manager.go:68显式Authorization优先验证，错误不回退Cookie。Frame处理器与service继续执行JPEG、设备存在和既有业务错误契约。 |
| A3 | passed | brief.md | A3：平台Basic能建立固定期限、有界Cookie会话，当前会话/退出行为明确；Cookie图片/WS可接入，失效/退出/重启撤销授权并关闭关联WS，设备不能取得平台会话。 | auth/manager.go:124使用独立随机会话/CSRF值、1024容量、固定期限及过期清理；manager.go:151、179实现撤销通知和规定Cookie属性。handler/auth.go:111注册平台会话POST/GET/DELETE，设备先被权限拒绝，关闭模式不注册。handler/websocket.go:41、58监听会话Done并关闭连接，释放订阅、ticker与128连接槽；server/app.go:156在关闭时撤销会话。 |
| A4 | passed | brief.md | A4：CORS、WS Origin和Cookie写保护一致；同源/明确许可同站不同端口可用，非法来源或CSRF拒绝，预检不触发业务；稳定图片/WS URL不附凭据。 | auth/origin.go:12、71统一配置及HTTP/WS的scheme、host、有效port判断，不信任转发头，拒绝非法、多个与未知来源。handler/auth.go:22正确回显许可来源、credentials及Vary；合法预检204且不进入业务，非法方法/请求头拒绝。auth.go:93要求Cookie写请求携带唯一匹配CSRF，Basic仍检查Origin。protocol/frame.go:46与README.md:98保留无凭据的图片/WS稳定URL。 |
| A5 | passed | brief.md | A5：Broker默认禁匿名，角色ACL覆盖五类上行/两类下行/设备遗嘱；设备不能发布或获得其他设备消息，健康账号无业务权限；凭据初始化及健康检查支持认证，保留retained/重连。 | deploy/mosquitto.conf:2默认禁匿名并引用密码/ACL文件。init-broker-auth.sh:16、38验证三个不同角色并生成显式user规则：平台五类上行读/两类下行写、设备自身反向权限、健康仅读$SYS/broker/uptime，包含config-acks和status遗嘱，无全局%u授权。docker-compose.yml:35使用健康账号认证检查；mqtt/client.go:69及subscriber.go:15、132保留初始失败退出、五类订阅与重连就绪边界。 |
| A6 | passed | brief.md | A6：模拟器独立MQTT/HTTP凭据及上传Basic接线正确，参数优先，缺失配对拒绝、不泄露、不重试认证拒绝；拍照/event-image/遥测/配置、重复/超时/取消保持。 | cmd/simulator/main.go:31、37、55提供独立MQTT/HTTP环境与参数，显式参数含空值优先，密码不进入帮助默认值，连接前验证设备绑定及配对。simulator/snapshot.go:37、134通过Basic请求头上传，根地址禁止凭据/非根路径且禁止重定向；snapshot.go:143使401/403终止重试。命令、event-image、遥测、配置及两种Ack接线保持，重试复用字节/身份/时间并受截止、总超时与取消约束。 |
| A7 | passed | brief.md | A7：Compose与原生启动所需凭据初始化、数据路径/卷、认证健康检查和依赖一致；模板无真实/固定密码，未初始化明确失败；README/网络规范包含可执行操作与延期运行边界。 | docker-compose.yml:2、19、42按初始化完成→认证Broker健康→后端启动配置依赖，认证、Broker数据与后端数据卷及路径一致，必填变量明确失败。deploy/init-broker-auth.sh:30、47仅替换管理文件并最后写ready，权限与容器归属一致；start-broker.sh:3拒绝未初始化。模板为空密码，.gitignore/.dockerignore排除真实.env和生成凭据。README.md:29—134及docs/adas_network_api_spec.md:2057涵盖原生/Compose、两组模拟器凭据、curl、浏览器会话/WS/图片/退出、轮换、失败定位和延期边界。 |
| A8 | passed | brief.md | A8：完整demo-delivery Spec、业务边界与文档一致；Go文件gofmt/go build ./...通过，新只读Verifier核对A1—A8；旧测试仅必要依赖机械适配，断言保留，不把未运行项写为通过。 | 完整R1—R9与A1—A8已重新独立核对，新增访问前置保留既有DTO、图片状态及业务接线；旧测试diff仅增加认证依赖，断言保留。本候选新增/修改Go文件gofmt -l无输出，两个脚本分别sh -n及git diff --check通过。.comet/runtime/native/changes/go-demo-delivery/state.json:4、22绑定当前candidate、stateVersion8及executionRef，go-build为Runtime证据、exit0。README.md:7、9明确未运行项与V1/V2，不将编译或示例当运行验收。 |

## 检查

| 检查 | 命令 | 工作目录 | 状态 | 退出码 | 耗时 |
| --- | --- | --- | --- | ---: | ---: |
| Go编译（修复后，测试后置） | build ./... | go-backend | passed | 0 | 8101 ms |

### Builder 报告的证据

以下为 Builder 报告，不等同于 Runtime 检查凭据或独立验收结果。

- 格式与差异: passed — 本轮origin.go gofmt及git diff --check通过；其他Go格式/两脚本sh -n沿用未改部分的开发期记录
- 当前候选Go编译: not-run — 源码修复使首轮编译不能代替当前候选，将由Runtime重新go build ./...
- 运行测试与环境联调: not-run — 用户明确延后test/race/vet、HTTP/MQTT、Broker/Docker与硬件，未执行
- 已知限制: 静态代码交付，动态拒绝、会话/WS、浏览器和Broker/Compose证据仍待V1/V2。
- 已知限制: 旧handler夹具仅认证依赖机械适配，断言保留；旧配置加载与匿名Broker集成夹具需V1更新。
- 已知限制: 跨站Cookie、公网TLS/代理、Vue/Python/硬件、用户管理及图片TTL不在本轮范围。

## 阻塞项

_无。_

## 风险与跳过的工作

- test/race/vet、实际HTTP/MQTT、浏览器会话与WS生命周期、Broker/Docker及硬件运行验证按已确认契约后置；本次通过仅表示代码、配置、文档与编译验收。
- 旧配置加载和匿名Broker集成夹具仍需V1更新；本轮保留旧断言。
- 全仓gofmt -l仍列出未修改的旧基线文件；本候选新增和修改Go文件均通过格式核对，未宣称旧基线已统一格式。

## 之前的迭代

| 目标周期 | 迭代 | 尝试 | 结果 | 未解决项 | 摘要 | 完成时间 |
| ---: | ---: | ---: | --- | --- | --- | --- |
| 1 | 1 | 1 | fail | A1, A4, A8 | 发现1个明确来源校验缺陷，影响A1、A4及完整Spec一致性A8；其余五项静态核对通过。先验证原authority的IPv6/方括号结构再归一化，可修复两个配置/请求入口。当前候选保持不变；未修改文件或状态，未提交，未运行延期测试或外部环境。 | 2026-10-10T09:04:12.222Z |
| 1 | 2 | 1 | pass | — | 当前iteration2/attempt1候选通过全部八项静态验收；原authority缺陷已修复，未发现新的阻断语义缺陷。仅执行只读检查，未修改文件、状态或提交，未运行后置测试及外部环境。 | 2026-10-10T09:14:09.498Z |



## 结论

当前iteration2/attempt1候选通过全部八项静态验收；原authority缺陷已修复，未发现新的阻断语义缺陷。仅执行只读检查，未修改文件、状态或提交，未运行后置测试及外部环境。
