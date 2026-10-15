# 演示接入、鉴权与部署完整目标规格

本capability统一HTTP/WS/图片与MQTT访问前置和演示部署。其他业务capability的字段、状态机、幂等、存储、过滤、QoS/retained和恢复行为在授权后继续生效；过去各业务change的鉴权后续范围由本规格接续。用户已确认本完整方案，本轮代码交付不代表运行验证通过。

## R1：默认模式、配置与凭据

默认配置与Compose开启HTTP鉴权，无内置默认密码。平台用户名/密码、设备凭据映射、会话TTL和额外origin支持YAML/环境变量，非空环境覆盖、相对数据路径规则保留。用户名为1—64 ASCII字母/数字/下划线/连字符；设备用户名即device_id，与平台用户名不冲突。密码非空且无换行/NUL，不自动注册设备。

默认需平台及seed.device_id设备凭据；缺失、错误类型、身份/账号冲突、非法origin或TTL在数据库/Broker初始化前拒绝，只报字段和原因，不回显密码。TTL缺省30分钟，允许1—1440分钟。HTTP显式关闭开关仅允许loopback监听127.0.0.1/::1/localhost；空凭据/验证异常/缺依赖不自动降级，关闭模式业务仍遵循来源限制，不注册会话接口。默认Compose不关闭HTTP鉴权。

MQTT沿用username/password，默认认证模式要求平台MQTT凭据完整，缺失/不配对也在外部初始化前拒绝。平台、设备和Broker健康角色分离，HTTP/MQTT密码分别配置，不按同名账号推导另一协议密码。.env.example仅空密码/填写说明，无固定密码；真实.env与生成密码文件被Git忽略，不输出密码、Authorization、Cookie、CSRF值。

## R2：HTTP身份、权限与失败

在处理器及其副作用前先检查Origin，再认证/授权；非法来源优先403，合法或无Origin后缺失/错误/失效身份401/code40101，有效身份无权限403/code40301。平台支持Basic或Cookie，设备仅支持绑定Basic。显式Authorization存在时必须验证，无效不回退Cookie/匿名；缺Authorization才查Cookie。拒绝不进入业务，Basic挑战不泄露账号是否存在；会话容量不足503/code50303，不冒充MQTT故障。

平台为唯一管理角色，可访问全部既有设备/状态/遥测/告警/配置/commands/frames/content/WS路由。设备只能POST /api/v1/devices/{自身device_id}/frames，不能跨设备上传或查询、控制、配置、读取图片、建立WS；合法上传仍执行既有JPEG/multipart/设备存在校验。health无需身份，合法预检不要求身份但不调用业务。

授权通过后未知设备/图片/命令、参数错误、离线/MQTT故障继续原HTTP信封、状态码和DTO。认证API/会话/图片响应设置Cache-Control:no-store，拒绝时不以旧响应代替鉴权。没有经数据库/文件/Hub副作用泄露内容的拒绝路径；auth配置不能代替Device登记，未知设备仍not found。

## R3：浏览器会话

启用认证时注册POST/GET/DELETE /api/v1/auth/session。POST仅平台Basic，成功201/code0及expires_at/csrf_token，设置不可猜随机Cookie；设备403、错误/缺身份401。Cookie为HttpOnly、host-only、Path=/api/v1、SameSite=Strict、固定TTL的Max-Age/Expires；直接HTTPS加Secure，课程HTTP可不加。会话规模有界，清理过期项但不延长活跃期限，满容量拒绝新建。

GET供平台读取身份/鉴权模式；Cookie身份返回当前expires_at/csrf_token，Basic无会话则为null、不自动设置Cookie。DELETE撤销请求所携有效会话并清除Cookie，返回204；Cookie调用须CSRF，平台Basic可直接调用。设备不能创建、查看或退出平台会话。关闭鉴权不注册这些接口或伪造登录。

会话固定到期，不滑动续期；退出/到期关闭相关WS并释放连接资源。重启不恢复会话，浏览器重新登录，原SQLite业务事实保留。没有会话/密码业务表改造、注册/找回密码、多管理角色、OAuth/JWT平台、SSO或分布式会话。

## R4：来源、CORS与Cookie写保护

目标拓扑为同一主机IP/域名和相同scheme，Vue可不同端口。默认仅后端同源；额外origin必须明确http(s) scheme/host/有效port，无用户信息、路径（根路径可规范化）、query/fragment、*/null或重复。真实origin由部署者配置，不把示例localhost:5173当既定事实。

有Origin时HTTP与WS用一致的同源/白名单判断，非法/null/多个/未知来源403；无Origin的CLI/设备仍须身份/权限。准确比较scheme/host/有效port，不做字符串包含或后缀授权，不默认信任任意X-Forwarded-*。

许可HTTP来源回显精确Access-Control-Allow-Origin、允许credentials并正确Vary。合法OPTIONS204，支持现有GET/POST/PUT/DELETE及Authorization/Content-Type/X-CSRF-Token；非法来源/方法/请求头拒绝，不执行业务，禁止credentials搭配*。浏览器fetch用credentials:include。

Cookie认证的POST/PUT/DELETE等写请求必须X-CSRF-Token匹配会话，包括命令、配置、平台上传、退出；缺失/错误403、无副作用。未依赖Cookie的Basic可直接调用，Origin仍检查。登录依赖显式Basic并检查Origin；GET不修改会话/业务。

跨站点主机/不同scheme Cookie兼容、TLS终止/反向代理、公网部署不在本轮范围。文档说明同站不同端口演示与真实白名单配置；来源许可不代替身份。

## R5：WS和稳定图片URL

WS升级前平台认证与Origin检查，之后才订阅Hub。浏览器用会话Cookie，非浏览器平台可Basic，不在URL附密码或长期token；设备/无身份/失效会话拒绝。会话到期/退出触发关联WS关闭，释放订阅/计时/连接；不新增并行写者，不持Monitor锁或长SQLite事务。

保留Hub有界队列/慢连接关闭/单写者/ping-pong，以及telemetry/event/event_image_update/device_status/command_update/frame_update；无config_update和历史补推。现有协议data不加入认证秘密。

content、设备frame、事件列表与其他查询一样需平台身份。浏览器登录后读取原稳定URL携Cookie，CLI平台Basic；snapshot_url、命令result.url、frame_update.url及告警六状态不变，无凭据参数或临时URL回写。设备只有上传权。未知/缺文件、同设备/类型校验、安全普通文件和持久幂等不变；有效图片状态不是匿名访问承诺。

## R6：MQTT账户与ACL

默认allow_anonymous=false，官方工具生成password_file，明确acl_file；未初始化不能静默匿名。平台、设备和健康账号身份合法、互不冲突；设备账号即device_id。ACL默认无权，使用显式角色规则，避免全局%u模式意外给平台/健康账号设备权。

| 角色 | 读 | 写 |
| --- | --- | --- |
| 平台 | car/+/telemetry、car/+/events、car/+/status、car/+/command-acks、car/+/config-acks | car/+/commands、car/+/config |
| 设备d | car/d/commands、car/d/config | car/d/telemetry、car/d/events、car/d/status、car/d/command-acks、car/d/config-acks |
| Broker健康 | $SYS/broker/uptime | 无 |

设备不能发布其他设备/commands/config/无关主题或获得其他设备消息；平台不能写上行/读下行/无关主题；健康无业务权。区分订阅申请返回与实际消息授权，不承诺不同Broker版本相同SUBACK错误表述。

status权限覆盖正常状态与离线遗嘱。保留五类上行、QoS、status/config retained和commands/Ack/event non-retained。平台启动/重连订阅与配置补发、旧传输隔离不变；认证失败不能算就绪。MQTT用户名/密码配对错误明确拒绝，不泄露密码。设备config-acks必须进入ACL示例。

## R7：软件设备

模拟器增加独立MQTT和HTTP用户名/密码环境/参数，显式参数优先。配对不完整/非法身份在运行前拒绝；两项皆空只用于显式本地开发配置/自备匿名Broker，不使正式部署降级。认证演示命令须提供两组设备凭据。

图片使用设备Basic请求头，不嵌-api URL，保留根地址约束/禁止重定向。camera.snapshot与-event-image共用上传身份，401/403终止上传、不重试认证拒绝，失败Ack/事件故障日志规则不变；临时网络故障有限重试仍复用时间/身份/原字节，命令截止、总超时与取消保持。

MQTT telemetry/status/events及两种Ack使用设备账号；commands/config订阅、遗嘱、执行缓存、有界队列及结束离线发布保留。HTTP失败不伪造图片、不阻塞MQTT回调/配置/遥测，软件JPEG不等于真实相机。

## R8：初始化、启动和文档

提供.env.example、被忽略的.env/生成凭据、初始化脚本或明确命令，用Mosquitto官方工具生成密码哈希；用户填写密码，无固定值。初始化失败不产生匿名降级/半成品成功标记，不覆盖无关文件。输出目录/权限与配置引用一致。

默认Compose先准备Broker凭据/ACL，再认证Broker健康，再启动后端；Broker专用健康账号，Go health无身份。Mosquitto配置/数据与backend-data卷/路径和权限相符，必填变量缺失明确失败，保留Go初始Broker失败退出。提供原生Go使用同一认证配置的步骤，不依赖Compose才能使用功能。

README包含填写凭据/origin、启动、模拟器、平台curl查询/命令/配置、设备上传、浏览器Basic登录/credentials fetch/WS/图片/退出及401/403/跨源/初始化失败定位。轮换需更新配置/文件/环境后重启并重新登录，无热更新/持久会话。示例不冒充实际运行证据。

同步网络规范第61/62节、401/403/容量错误码和所有操作示例，补齐config-acks，清理匿名默认/鉴权尚未实现的现状表述。其他业务Spec非目标限定各自capability，本规格提供新访问前置，无需改业务状态机。

## R9：交付与后验

brief A1—A8核对代码、配置和文档；gofmt/go build ./...及新独立只读验收。旧测试仅必要依赖机械适配、断言保留；不新增或运行test/race/vet、HTTP/MQTT、Broker/Docker、双平台/硬件。

V1更新旧夹具/契约断言，验证拒绝无副作用/角色/会话/CSRF/CORS/WS生命周期及全业务回归，再test/vet/race。V2实际Broker多设备ACL/遗嘱/重连/retained、Compose、完整网络、Vue/Python/相机/树莓派；没有证据不写运行通过。图片清理/TTL/删除、命令历史、视频/WS outbox不在本轮。

## 事实参考

- 项目网络规范第61/62节：HTTP Basic、MQTT账号与同设备Topic隔离。
- [WHATWG WebSocket握手](https://websockets.spec.whatwg.org/#opening-handshake)：credentials mode为include，构造接口URL/protocols；方案采用Cookie。
- [WHATWG Fetch CORS](https://fetch.spec.whatwg.org/#http-cors-protocol)：带凭据跨源使用明确来源及credentials。
- [Mosquitto配置手册](https://mosquitto.org/man/mosquitto-conf-5.html)：password_file、acl_file、主题角色与匿名开关。
