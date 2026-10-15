# Outcome

补齐 Go 后端剩余的演示交付功能：HTTP/WS/图片鉴权、设备权限边界、MQTT 用户/ACL、前端跨源配置及启动与操作示例。承接当前约95%的功能代码口径；完成后再统一进行V1软件测试与V2环境联调。功能代码完成不等于运行验收通过。

# Scope

- 默认开启HTTP鉴权，必要凭据缺失时在数据库/Broker初始化前退出，无内置默认密码。显式关闭HTTP鉴权的开发模式只允许loopback；默认Compose使用认证Broker，禁用匿名。
- 平台账号通过Basic访问全部业务API。设备用户名即device_id，仅可POST自己的frames，不能查询、控制、改配置、读图片或连WS。health无需凭据；认证失败401/40101，权限或来源拒绝403/40301。
- 浏览器用平台Basic建立短期Cookie会话，缺省30分钟固定到期，提供登录/当前会话/退出接口；HttpOnly、host-only、Path=/api/v1、SameSite=Strict，直接HTTPS时Secure。退出/到期/重启使会话失效并关闭关联WS，会话规模有界。
- 演示拓扑是同一主机IP/域名、相同scheme，Vue可不同端口；真实origin通过配置给出，不把localhost:5173当既定事实。HTTP CORS与WS Origin共用明确白名单，默认仅同源，拒绝*/null/非法来源；无Origin的CLI/设备仍须认证授权。
- 浏览器fetch使用credentials:include，WS/图片使用会话；Cookie写请求必须带CSRF token。Basic客户端可直接调用，Origin仍检查。稳定图片/WS URL不附加密码或长期token，不改变已接受DTO与图片状态。
- Mosquitto分离平台、设备和健康账号：平台读五类上行、写commands/config；设备写自己的telemetry/events/status/command-acks/config-acks、读自己的commands/config；健康账号只读$SYS/broker/uptime。保留QoS、retained、遗嘱与重连，不增加设备注册。
- 模拟器增加独立MQTT/HTTP凭据环境变量及参数，图片上传使用绑定设备Basic；参数优先，凭据不进入URL/日志，保留拍照/event-image/遥测/配置、重复/超时/取消行为。
- 更新config.yaml、Compose、Mosquitto配置、凭据初始化、.env.example/忽略规则、README/网络规范。提供原生Go/Compose、curl、浏览器登录/WS/图片、模拟器、轮换与失败定位说明；本轮不启动外部环境。

实施顺序：先完成配置校验及HTTP Basic角色边界（A1/A2），再接会话、WS生命周期和来源/CSRF（A3/A4），接着补Broker角色及模拟器凭据（A5/A6），最后整理初始化/部署/文档并统一代码验收（A7/A8）。这是同一change的内部顺序，每步都承接前一步的接入契约。

# Non-goals

- 不新增或运行test/race/vet、HTTP/MQTT链路、Broker/Docker、双平台、Python/Vue/硬件测试，也不先安装或排查验证环境。
- 不开发Vue页面、Python感知/规则/相机硬件；不增加用户注册/找回密码、OAuth/JWT平台、细粒度多租户、自动设备注册、分布式/持久会话或SSO。
- 支持课程本机/局域网拓扑，公网/TLS证书/反向代理、跨站点浏览器Cookie兼容另行规划。
- 不增加图片TTL/删除/容量清理、命令历史、视频、持久WS outbox或告警实例状态机，不扩大已接受业务契约。
- 不推送/部署/合并其他分支，不修改旧归档Runtime。

# Acceptance examples

- A1：默认认证与环境覆盖正确；平台/设备身份、凭据配对/冲突、TTL/origin缺失或非法在外部初始化前拒绝；显式匿名HTTP仅loopback，无默认密码或凭据日志/提交。
- A2：health/合法预检之外业务统一认证授权；平台可操作原业务，设备仅上传绑定设备帧，跨设备/API/WS/图片拒绝；401/403无副作用，不回退匿名或另一凭据。
- A3：平台Basic能建立固定期限、有界Cookie会话，当前会话/退出行为明确；Cookie图片/WS可接入，失效/退出/重启撤销授权并关闭关联WS，设备不能取得平台会话。
- A4：CORS、WS Origin和Cookie写保护一致；同源/明确许可同站不同端口可用，非法来源或CSRF拒绝，预检不触发业务；稳定图片/WS URL不附凭据。
- A5：Broker默认禁匿名，角色ACL覆盖五类上行/两类下行/设备遗嘱；设备不能发布或获得其他设备消息，健康账号无业务权限；凭据初始化及健康检查支持认证，保留retained/重连。
- A6：模拟器独立MQTT/HTTP凭据及上传Basic接线正确，参数优先，缺失配对拒绝、不泄露、不重试认证拒绝；拍照/event-image/遥测/配置、重复/超时/取消保持。
- A7：Compose与原生启动所需凭据初始化、数据路径/卷、认证健康检查和依赖一致；模板无真实/固定密码，未初始化明确失败；README/网络规范包含可执行操作与延期运行边界。
- A8：完整demo-delivery Spec、业务边界与文档一致；Go文件gofmt/go build ./...通过，新只读Verifier核对A1—A8；旧测试仅必要依赖机械适配，断言保留，不把未运行项写为通过。

# Constraints and invariants

- 在既有Gin/WS/配置/模拟器接线访问控制，不重写Command/Event/Frame/Config/Monitor/Hub。身份/来源拒绝发生在DB写入、MQTT发布、文件写入与Hub订阅前。
- 保留拍照quality/有效Ack/终态、告警原引用幂等/六状态/更新次序、图片安全发布/预览排序、配置版本/retained恢复及监控在线/freshness。
- 凭据配置不代替Device注册；未知设备仍按原业务not found。平台为唯一管理角色，设备权限绑定device_id。
- 会话/连接/队列有界，不持Monitor锁或长SQLite事务处理认证。跨能力访问前置由demo-delivery定义，各业务capability继续定义授权后的业务行为。
- 95%是功能权重估算，不含Vue/Python/硬件，也不是覆盖率或运行通过比例；F3后仍有V1/V2。

# Decisions

- 用户于2026-10-10接受/归档告警图片结果，代码e37463f、归档bb81bcf；当前干净，沿用go后端开发/current。
- 用户要求规划并完成剩余功能，测试后置延续；HTTP Basic和同设备Topic隔离来自网络规范第61/62节，设备config-acks须补入ACL例子。
- 推荐默认鉴权/认证Broker，仅显式loopback开发可关闭HTTP；平台Basic加30分钟可退出Cookie会话，保留稳定图片URL；设备仅上传自己帧，health无凭据。
- 推荐同主机、同scheme、不同端口局域网方案和明确origin/CSRF；仓库无Vue工程或真实前端origin，示例端口不等于部署事实。
- 拆分检测：鉴权、来源/CSRF、模拟器凭据和部署/健康共同形成一个接入结果，会反复改同一配置/路由/模拟器/文档；保持单一Native change，按依赖顺序执行，不创建Supervisor/并行子change。
- 用户于2026-10-10回复“同意”，确认本完整方案的目标、范围、关键决定、A1—A8验收及全部非目标，授权进入Build实施。

# Open questions

- 无未解决问题；普通实现选择在已确认契约内处理。

# Verification expectations

- Shape只调查事实/编辑正式方案；明确确认完整目标、范围、关键决定、验收及非目标后移除CONFIRM，通过Runtime进入Build。
- Build仅格式/编译/代码配置核对；Runtime只go build ./...，新只读Verifier核对A1—A8，不把延期测试本身当失败，不以编译代替语义核对。
- V1在F1—F3代码完成后更新旧夹具/契约断言，补齐认证/角色/会话/CSRF/CORS/WS生命周期及业务回归，再test/vet/race；V2真实Broker ACL/Compose/完整网络/Vue/Python/硬件。
