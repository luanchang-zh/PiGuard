# Go 监控与控制后端

在 `go-backend` 目录运行。已实现 telemetry/status MQTT 接入、最新状态、历史查询、动作/拍照命令与 Ack、告警与图片关联、配置同步、JPEG 上传/读取、WebSocket、认证接入和部署。无硬件软件验收使用真实 SQLite、HTTP、WS、认证 Mosquitto 与软件设备；真实传感器、GPIO 和相机留待硬件验收。

## 软件验收

验收按三步执行：V1 更新旧契约断言并完成软件回归；V2 使用认证 Compose 验证真实网络、设备 ACL 和故障恢复；V3 使用真实浏览器验证会话，并持续运行 30 分钟。结果分别记录通过、失败、环境阻塞和硬件待验。

新增测试覆盖默认认证配置、角色拒绝无副作用、Authorization 优先、固定期限/容量/撤销、CORS/CSRF/预检、Cookie WS、拍照真实 HTTP 上传→Ack→双 WS、错误图片引用、迟到回执、丢失响应重试、取消，以及告警图片双顺序/并发幂等/存储故障/重启。

```sh
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
```

默认测试使用临时 SQLite、模拟发布器、软件设备执行器、HTTP/WS 客户端及 Paho/net.Pipe MQTT 传输，无需 Docker 或硬件。旧业务夹具显式使用 loopback 开发认证，新增验收夹具开启认证。Windows 缺少符号链接权限时只跳过对应子用例，其余故障断言继续执行；该用例由 Linux 容器实际覆盖。

软件回归、Docker 和持续运行验收在 Bash/Git Bash 运行；浏览器检查按下文步骤单独触发：

```sh
bash scripts/verify-software.sh
```

脚本执行本机 test/vet、Linux 容器 test/vet/race，以及认证 Compose 和默认 30 分钟持续运行。日志保存在被忽略的 `data/acceptance/`。Docker 应已启动，容器安装编译器需要网络。本机竞态环境不可用时使用 Linux 结果，并如实保留平台差异。

仅运行网络验收（不含持续运行）：

```sh
go test -tags integration ./internal/server -run '^TestDockerAcceptance$' -count=1 -timeout 50m -v
```

增加持续运行：

```sh
PIGUARD_SOAK_DURATION=30m go test -tags integration ./internal/server -run '^TestDockerAcceptance$' -count=1 -timeout 50m -v
```

网络夹具读取现有 Compose，创建独立项目、随机凭据和专属数据卷，在 loopback 分配端口并保持本次重启前后一致。它通过真实 seed 流程登记第二台设备，为其生成明确 ACL；不读取或覆盖开发者 `.env`。结束时只删除本次项目的容器/卷。检查包含两台软件设备、HTTP/两路 WS、五类命令、配置成功/失败/离线补发、JPEG、实际跨设备发布/订阅拒绝、遗嘱、Broker/后端重启、历史/图片/命令持久性及日志凭据检查。持续运行反复执行命令、拍照、WS 重连，并记录容器内存、线程数和文件描述符。

持续运行开始时会输出一次性浏览器验收页面 URL。使用 Playwright CLI 打开该 URL，执行 `run-code --filename scripts/browser-auth-control.js`，然后从 snapshot 点击“开始检查”。页面检查实际 Cookie/CORS/CSRF、拍照 JPEG、Cookie WS、退出和固定期限到期。控制脚本取消 Chrome 的 HTTP Basic 认证弹窗，使负向请求返回实际 401。页面只在测试期间提供临时凭据，不保存认证状态文件或包含凭据的网络 trace。

```sh
npx --package @playwright/cli playwright-cli -s=piguard open 浏览器验收URL --browser chrome
npx --package @playwright/cli playwright-cli -s=piguard run-code --filename scripts/browser-auth-control.js
npx --package @playwright/cli playwright-cli -s=piguard snapshot
```

2026-10-11 本轮软件验收通过：Windows test/vet、Linux test/vet/race、认证 Docker 网络与重启恢复，以及 Chrome 的 9 项检查。30 分钟持续运行完成 816 次命令/WS 重连循环和额外 81 次拍照；内存最大读数 21.52 MiB，末段 20.58—21.52 MiB，文件描述符 11—14。测试容器/卷及浏览器已清理。结果和日志见 `data/acceptance/report.json`；该短时软件结果不替代真实硬件、硬断电或长期运行验收。

Windows 的竞态检查需要启用 CGo，并使用包含 MinGW-w64 8 或更新运行库的 C 编译器，见 [Go 官方要求](https://go.dev/doc/articles/race_detector#Requirements)。使用便携工具链时，可在 Git Bash 中仅为本次检查指定编译器：

```sh
CC='C:/path/to/w64devkit/bin/gcc.exe' go test -race -count=1 ./...
```

## 认证配置与启动

默认开启 HTTP 认证和 Broker 认证，没有可直接使用的默认密码。先 `cp .env.example .env`，自行填写五个非空密码（平台 HTTP、设备 HTTP、平台 MQTT、设备 MQTT、Broker 健康）；HTTP 与 MQTT 独立。用户名为 1—64 位 ASCII 字母、数字、下划线或连字符。设备用户名就是 `PIGUARD_SEED_DEVICE_ID`，MQTT 平台/设备/健康身份不得重用；HTTP 平台与设备也不能同名。密码允许特殊字符，禁止换行/NUL；在 `.env` 中用单引号保护 `$` 等字符，Bash 原生启动需要兼容的引号语法。

`PIGUARD_HTTP_ALLOWED_ORIGINS` 为 JSON 数组，例如 `'["http://127.0.0.1:5173"]'`；5173 只是例子，改成实际 Vue 来源。默认 `[]` 仅后端同源。浏览器与后端应使用同一个 IP/域名和同一 HTTP(S) scheme，端口可不同。`localhost` 与 `127.0.0.1` 是不同主机，不能混用来演示 Cookie。跨站点、TLS 证书和代理部署另行处理。

```sh
docker compose up --build
```

Compose 先运行 `broker-init`，用官方 `mosquitto_passwd` 生成哈希密码和显式 ACL，再等认证 Broker 健康，最后启动 Go。未填必需变量时 Compose 直接拒绝；初始化失败不会启动匿名 Broker。`broker-auth` 保存认证文件，`broker-data` 保存 Broker persistence，`backend-data` 保存 SQLite/图片。初始化只替换认证目录中的 passwords/acl/ready；不要编辑生成的密码哈希。健康账号只读 `$SYS/broker/uptime`，后端 `/health` 无需凭据。后端首次连接 Broker 失败仍退出。

生成文件权限为目录 750、密码/ACL 640，容器内归 mosquitto 所有；Broker 只读挂载认证卷，启动脚本仅调整数据卷权限。官方镜像布局参考 [Mosquitto Dockerfile](https://github.com/eclipse-mosquitto/mosquitto/blob/v2.0.21/docker/2.0/Dockerfile)。认证 Compose 的软件运行验证见上面的验收流程。

原生 Go 不依赖 Compose。先安装 Mosquitto（含 `mosquitto_passwd`），在 Bash/Git Bash 导入自己填写的 `.env`，使用同一套账号初始化：

```sh
set -a
. ./.env
set +a
auth_dir="$(pwd)/deploy/generated-auth"
sh deploy/init-broker-auth.sh "$auth_dir"
mkdir -p "$auth_dir/data"
printf 'listener 1883 127.0.0.1\nallow_anonymous false\npassword_file %s/passwords\nacl_file %s/acl\npersistence true\npersistence_location %s/data/\nlog_dest stdout\n' \
  "$auth_dir" "$auth_dir" "$auth_dir" > "$auth_dir/mosquitto.conf"
mosquitto -c "$auth_dir/mosquitto.conf"
```

另开终端，同样导入 `.env` 后启动 Go：

```sh
go run ./cmd/server -config config.yaml
```

原生 Broker 示例绑定 loopback；局域网接设备时调整 listener，Go 使用 `PIGUARD_HTTP_ADDR=0.0.0.0:8080`，保持认证开启。Linux 原生 Broker 用当前普通用户运行，或为实际服务用户设置认证目录和数据目录读取/写入权限；Windows 使用同一用户并确认文件 ACL。原生配置和 Compose 引用的文件位置不同，上面的命令会生成与本机绝对路径一致的配置。原生 Broker 启动前须确认初始化成功及 ready 存在。

HTTP 其他配置可写在 `config.yaml` 的 `http.auth`：disabled、platform_username/password、device_passwords 映射、session_ttl_minutes（缺省 30，1—1440）、allowed_origins。对应环境变量为 `PIGUARD_HTTP_AUTH_DISABLED`、`PIGUARD_HTTP_PLATFORM_USERNAME/PASSWORD`、`PIGUARD_HTTP_DEVICE_PASSWORDS`（JSON 对象）、`PIGUARD_HTTP_SESSION_TTL_MINUTES` 和 `PIGUARD_HTTP_ALLOWED_ORIGINS`（JSON 数组）。非空环境覆盖 YAML；`PIGUARD_HTTP_SEED_DEVICE_PASSWORD` 最后覆盖 seed 身份的密码，方便单设备演示。更换 seed 时同步设备端用户名；多设备用完整 JSON 映射配置各自密码。配置相对数据路径以配置文件目录为基准；非法配置在建目录/数据库/连接 Broker 之前退出，错误不回显凭据。

仅显式本地开发可设 `PIGUARD_HTTP_AUTH_DISABLED=true`，监听必须为 127.0.0.1/::1/localhost；来源检查仍开启，会话路由不注册。默认 Compose 不提供匿名模式。独立自备匿名 Broker 的模拟器必须显式加 `-allow-anonymous`，不会因空凭据自动降级。

## 平台与设备接入

平台 Basic 可访问全部现有业务 API；设备 Basic 只可 POST 自己的 frames，不能读设备列表/图片、控制、配置或连 WS。凭据不注册设备，未知身份对应的业务设备仍须先登记。先导入 `.env`，再将模拟器设备凭据导出：

```sh
export PIGUARD_SIM_MQTT_USERNAME="$PIGUARD_SEED_DEVICE_ID"
export PIGUARD_SIM_MQTT_PASSWORD="$PIGUARD_DEVICE_MQTT_PASSWORD"
export PIGUARD_SIM_HTTP_USERNAME="$PIGUARD_SEED_DEVICE_ID"
export PIGUARD_SIM_HTTP_PASSWORD="$PIGUARD_HTTP_SEED_DEVICE_PASSWORD"
go run ./cmd/simulator -device "$PIGUARD_SEED_DEVICE_ID" -duration 30s
```

MQTT/HTTP 独立参数为 `-mqtt-username/-mqtt-password/-http-username/-http-password`，显式参数优先于上述环境变量（显式空值也会覆盖）。配对不完整、用户名不等于 device 或非法凭据在连接之前拒绝；密码推荐环境变量，帮助文本不显示环境密码。模拟器正常结束发布离线状态，异常断连使用离线遗嘱；`-broker tcp://host:1883` 与 `-api http://host:8080` 独立配置，URL 不放凭据，未知设备不自动注册。

平台查询：

```sh
curl -u "$PIGUARD_HTTP_PLATFORM_USERNAME:$PIGUARD_HTTP_PLATFORM_PASSWORD" http://127.0.0.1:8080/api/v1/devices
curl -u "$PIGUARD_HTTP_PLATFORM_USERNAME:$PIGUARD_HTTP_PLATFORM_PASSWORD" http://127.0.0.1:8080/api/v1/devices/car-001
curl -u "$PIGUARD_HTTP_PLATFORM_USERNAME:$PIGUARD_HTTP_PLATFORM_PASSWORD" http://127.0.0.1:8080/api/v1/devices/car-001/state
curl -u "$PIGUARD_HTTP_PLATFORM_USERNAME:$PIGUARD_HTTP_PLATFORM_PASSWORD" 'http://127.0.0.1:8080/api/v1/devices/car-001/telemetry?limit=10'
```

以下示例使用默认已登记 `car-001`，更改 seed 时替换 URL。所有查询/写入先验证来源和身份；显式 Authorization 错误不会回退 Cookie。业务/API/图片响应 `Cache-Control: no-store`。MQTT ACL 默认拒绝：平台读 telemetry/events/status/command-acks/config-acks、写 commands/config；设备只写自己的五类上行（含 status 遗嘱），只读自己的两类下行。没有全局 `%u` 权限规则。增加设备需显式登记并增加其 HTTP 凭据、Broker 密码和独立 user ACL，本轮没有注册 API。

## 浏览器登录、WS 与图片

在真实 Vue 来源页面的控制台演示，先将该来源加入白名单；保持同主机同 scheme。登录 POST 必须平台 Basic，成功 201，Cookie HttpOnly/host-only/Path=/api/v1/SameSite=Strict；直接 HTTPS 才加 Secure。会话固定到期、不滑动延长，缺省 30 分钟，最多 1024 份；退出、到期和重启失效并关闭关联 WS。WS 同时最多 128 条连接。密码、Cookie、CSRF 不放 URL 或打印日志。

```js
const api = `http://${location.hostname}:8080`; // 改为实际后端，保留同主机和 scheme
const username = prompt('平台用户名');
const password = prompt('平台密码');
const bytes = new TextEncoder().encode(`${username}:${password}`);
const basic = btoa(String.fromCharCode(...bytes));
const login = await fetch(`${api}/api/v1/auth/session`, {
  method: 'POST', credentials: 'include', headers: {Authorization: `Basic ${basic}`}
});
if (!login.ok) throw new Error(`登录失败 HTTP ${login.status}`);
const session = (await login.json()).data; // expires_at / csrf_token，不打印秘密
const current = await fetch(`${api}/api/v1/auth/session`, {credentials: 'include'});
if (!current.ok) throw new Error('会话已失效，需要重新登录');
const wsURL = new URL('/api/v1/ws', api);
wsURL.protocol = wsURL.protocol === 'https:' ? 'wss:' : 'ws:';
const ws = new WebSocket(wsURL); // 浏览器自动携 Cookie
ws.onmessage = event => console.log(JSON.parse(event.data));
const response = await fetch(`${api}/api/v1/devices/car-001/frame`, {credentials: 'include'});
if (response.ok) {
  const frame = (await response.json()).data;
  const img = new Image(); img.src = new URL(frame.url, api); document.body.append(img);
}
// Cookie 写请求：命令、配置、平台上传和退出均加 X-CSRF-Token。
const logout = await fetch(`${api}/api/v1/auth/session`, {
  method: 'DELETE', credentials: 'include', headers: {'X-CSRF-Token': session.csrf_token}
});
if (!logout.ok) throw new Error(`退出失败 HTTP ${logout.status}`);
ws.close();
```

GET session 返回 username/role/auth_mode；Cookie 返回 expires_at/csrf_token，Basic 不自动创建会话，未携有效会话则两字段 null。DELETE 为 204，撤销所携有效会话并清 Cookie；平台 Basic 也可直接退出。设备不能创建、查询或退出会话。原 snapshot_url/result.url/frame_update.url 保持稳定，图片读取用平台 Basic 或登录 Cookie，不加临时凭据参数。

来源检查先于身份。合法 OPTIONS 预检无需身份，支持 GET/POST/PUT/DELETE 和 Authorization/Content-Type/X-CSRF-Token，204 且无业务副作用；明确回显许可来源和 credentials，不用 `*`。Cookie 写请求缺少/错误 CSRF 返回 403；Basic 不依赖 Cookie，因此无需 CSRF，仍检查 Origin。

失败定位：401/40101 是凭据缺失/错误、Cookie 到期或重启失效；403/40301 是权限/Origin/CSRF 拒绝（非法来源先拒绝），检查浏览器实际 scheme、主机、端口及 credentials。503/50303 是会话容量用尽；WS 容量拒绝为 503/50001。Broker 不健康先看初始化结果、身份冲突、文件权限、健康账号密码与 ACL，不改为匿名。日志不记录 Authorization/Cookie/CSRF/密码。

轮换时停服务，修改 `.env`/YAML/环境和认证生成源，重新初始化后重启 Broker/Go；Compose 可用 `docker compose down`（不加 `-v`）再 `docker compose up --build --force-recreate`。原生停止 Broker/Go 后重新运行初始化脚本和启动命令。保留数据卷/SQLite，浏览器重新登录，设备同时更新两组密码。没有凭据热更新或持久会话。

## 动作命令

模拟器在线后可创建命令：

```sh
curl -u "$PIGUARD_HTTP_PLATFORM_USERNAME:$PIGUARD_HTTP_PLATFORM_PASSWORD" -X POST http://127.0.0.1:8080/api/v1/devices/car-001/commands \
  -H 'Content-Type: application/json' \
  -d '{"type":"buzzer.test","params":{"duration_ms":1000}}'
curl -u "$PIGUARD_HTTP_PLATFORM_USERNAME:$PIGUARD_HTTP_PLATFORM_PASSWORD" http://127.0.0.1:8080/api/v1/commands/返回的command_id
```

- 四类动作：buzzer.test、indicator.test、scenario.start、scenario.stop，另支持下述 camera.snapshot。蜂鸣器/灯时长 100—5000ms，灯色 green/yellow/red；场景名称与播放速度遵循网络规范。配置使用独立接口。
- POST 返回 202/pending，表示已保存并异步受理；MQTT QoS 1、non-retained。Broker 确认后为 sent，设备回执决定 success/failed；10 秒内未收到有效回执则 timeout。
- GET 和 WS command_update 使用同一 DTO，包括参数、签发/截止、平台 ack_at、设备 executed_at 和 result/error。缺少回执的时间为 null。
- 离线设备拒绝为 503/50301；在线但发布通道不可用为 503/50302。每次有效 POST 创建新命令，没有 HTTP 重试幂等键。
- 快速/重复/冲突/迟到 Ack 不回退终态。重启保留记录、恢复截止时间，不自动重发动作。
- 模拟器 `-ack-mode success|failed|none` 选择结果，`-ack-delay 12s` 可模拟迟到回执；它按 command_id 缓存结果，重复投递只重发回执，不重复模拟执行。

## 拍照命令

在 Broker/后端已启动的环境中，启动软件设备，再用另一个终端创建命令：

```sh
go run ./cmd/simulator -duration 60s -api http://127.0.0.1:8080
```

```sh
curl -u "$PIGUARD_HTTP_PLATFORM_USERNAME:$PIGUARD_HTTP_PLATFORM_PASSWORD" -X POST http://127.0.0.1:8080/api/v1/devices/car-001/commands \
  -H 'Content-Type: application/json' \
  -d '{"type":"camera.snapshot","params":{"quality":80}}'
curl -u "$PIGUARD_HTTP_PLATFORM_USERNAME:$PIGUARD_HTTP_PLATFORM_PASSWORD" http://127.0.0.1:8080/api/v1/commands/返回的command_id
curl -u "$PIGUARD_HTTP_PLATFORM_USERNAME:$PIGUARD_HTTP_PLATFORM_PASSWORD" http://127.0.0.1:8080/api/v1/snapshots/成功结果中的snapshot_id/content -o snapshot.jpg
```

- `params` 必须为对象，quality 可以省略，缺省 80；显式值为 1—100 整数。null、额外字段、非整数和越界返回 400/40001。平台在保存/下发时补齐默认 quality。
- 软件设备在 success 模式生成 320×180 JPEG，按 snapshot 类型上传既有 frames API，然后用服务端返回的 snapshot_id 发 Ack；这是软件图片，不是实际摄像头拍摄。`-api` 指向 HTTP 根地址，`-broker` 独立选择 MQTT 地址。
- MQTT 回调只将命令放入有界队列，独立工作过程上传；整个上传受命令截止和进程取消约束，每次 HTTP 最多 6 秒。网络/响应读取故障、408/429/5xx 最多尝试 3 次，间隔 100ms，使用同一 frame_id、captured_at 和字节；其他拒绝不重试。上传失败发 SNAPSHOT_UPLOAD_FAILED，不编造图片 ID。failed/none 模式不上传，分别产生失败 Ack/等待超时。
- 拍照 success Ack 的 result 严格只含 snapshot_id。平台查已提交记录，要求同设备、type=snapshot 且文件可打开；未知、跨设备、preview/event、缺文件或非法 result 不结束命令、不发 command_update，继续等有效回执或 timeout。不以设备图片时间判断 Ack 是否准时。
- GET/command_update 的 success result 为 `{"snapshot_id":"snap-...","url":"/api/v1/snapshots/snap-.../content"}`。原设备 result 保留用于重复判断；首个终态不会被重复、冲突、迟到回执覆盖。图片上传的 frame_update 与命令状态更新分别产生，上传本身不结束命令。
- 已提交图片不会因命令超时或迟到回执删除；之后文件缺失由 content API 返回 404，历史命令仍保留原成功事实。平台不要求同设备图片只能被一条命令引用，软件设备每条新命令仍生成一张新图。
- 软件测试覆盖 HTTP→模拟器上传→Ack→GET/双 WS；真实认证 Broker/Compose 链路由 Docker 验收夹具检查，真实相机仍待硬件验证。

## 告警与图片

```sh
go run ./cmd/simulator -duration 60s -event-image -api http://127.0.0.1:8080
curl -u "$PIGUARD_HTTP_PLATFORM_USERNAME:$PIGUARD_HTTP_PLATFORM_PASSWORD" 'http://127.0.0.1:8080/api/v1/devices/car-001/events?limit=10'
```

- 告警上行可省略图片引用，或使用互斥的 `snapshot_id` / `frame_id`。snapshot_id 是 1—64 个非空白字符；frame_id 沿用上传身份的 1—64 ASCII 字母/数字/下划线/连字符，支持先告警后图片。显式 null、非法类型、同时提供两种引用均拒绝。合法告警即使引用错误或图片未到也保存。
- SQLite 原始引用不可变，参与 event_id 重复比较；关联结果不回写原引用，设备 snapshot_available 仅为兼容输入。重复事件不新增或推送，冲突保留首条。
- HTTP items、WS event 与 event_image_update 一致包含 `frame_id/snapshot_id/snapshot_available/snapshot_url/snapshot_state`。frame_id 保留输入；snapshot_id 与稳定 `/api/v1/snapshots/{id}/content` URL 只有同设备、type=snapshot/event、已提交且普通文件可打开时提供，其余为 null。
- 状态为 none（无引用）、pending（尚无图片记录）、available（可读）、invalid（跨设备/preview）、missing（有记录但缺文件）、unavailable（通知时暂时读取故障）。GET 遇到 DB/其他存储故障返回 500；告警保存后的通知解析故障仍发 event，图片为 unavailable，并记录诊断。
- 图片先到，首次 event 即可 available；告警先到，新图片提交后发 `event_image_update`，data 只含 event_id/device_id 和图片字段，前端据 event_id 更新原告警。该通知不创建另一条告警，不重发 event；幂等上传不重复通知。新 preview 可使相关引用更新为 invalid。GET 在重启后仍从持久引用和文件解析，WS 不补推历史；外部删图后 GET 反映 missing。
- `-event-image` 默认关闭，保留无图演示。启用后先发布两次相同内容、带稳定 frame_id 的告警，再在独立过程上传 320×180 软件 event JPEG。HTTP 使用 -api，最多三次复用相同身份/时间/字节，整体 10 秒且每次最多 6 秒，进程结束会取消并等待；失败只记录日志，不伪造图片可用。遥测、配置和拍照工作继续运行。MQTT 与 HTTP 接收顺序仍由真实网络决定，平台支持两种顺序。
- 当前 WS 类型为 telemetry/event/event_image_update/device_status/command_update/frame_update。配置状态通过 GET 查询，没有 config_update；软件回归和真实 Broker/Compose 证据见验收章节。

## 配置同步

```sh
curl -u "$PIGUARD_HTTP_PLATFORM_USERNAME:$PIGUARD_HTTP_PLATFORM_PASSWORD" http://127.0.0.1:8080/api/v1/devices/car-001/config
curl -u "$PIGUARD_HTTP_PLATFORM_USERNAME:$PIGUARD_HTTP_PLATFORM_PASSWORD" -X PUT http://127.0.0.1:8080/api/v1/devices/car-001/config \
  -H 'Content-Type: application/json' \
  -d '{"expected_version":1,"rules":{"obstacle":{"warning_distance_m":20}}}'
```

- PUT 的 expected_version 使用刚读取的 desired_version；旧版本返回 409/40902。支持部分规则字段，合并后校验完整四类规则；未知字段、null、空规则或不合法的阈值返回 400/40001 和字段路径。每次有效受理创建下一版本，包括值相同的请求。
- HTTP 202/pending 表示完整规则和版本已持久保存。设备离线仍可提交；MQTT 不可用则在写入前返回 503/50302。异步发送失败或结果不确定保持 pending，等待重连或重启补发。
- 配置独立发布到 car/{device_id}/config，QoS 1、retained=true。合法 config-acks success 才推进 reported_version；failed 保存设备 error，保留上一成功版本。配置不采用动作命令的 10 秒截止。
- GET 返回完整期望 rules、desired_version、Ack 确认的 reported_version、当前版本的 pending/success/failed、ack_at、applied_at、error；没有回执的字段为 null。通过 GET 查询结果，本轮没有 config_update WS 类型。
- 设备接口的 config_version 仍是 status 报告，和配置 Ack 确认事实分开。每版本首个有效结果保持不变，旧版本 Ack 不改变当前版本结果或回退已确认版本。
- 平台启动及 Broker 重连先恢复五组上行订阅，再补发每设备最新配置，版本和 issued_at 不变。即使 Broker 未持久保存 retained 数据，也可从 SQLite 恢复。补发不触发动作命令重发。
- 每次重连使用新的 clean session，丢弃旧 MQTT 客户端的未确认发送队列；只有五组订阅获得预期 QoS 后才恢复发布。发布不确定或取消会先关闭旧传输，再建立新会话，避免旧 retained 配置晚于新配置重放。
- 模拟器新增 `-config-ack-mode success|failed|none` 与 `-config-ack-delay 2s`。它按设备/版本缓存结果；重复配置只重发回执，同版本内容冲突和旧版本不回退规则。none 会应用配置但不回 Ack，用于模拟确认丢失。

## 图片上传与读取

```sh
curl -u "$PIGUARD_SIM_HTTP_USERNAME:$PIGUARD_SIM_HTTP_PASSWORD" -X POST http://127.0.0.1:8080/api/v1/devices/car-001/frames \
  -F 'file=@preview.jpg' \
  -F 'frame_id=preview-001' \
  -F 'captured_at=2026-10-09T12:00:00.123Z' \
  -F 'type=preview'
curl -u "$PIGUARD_HTTP_PLATFORM_USERNAME:$PIGUARD_HTTP_PLATFORM_PASSWORD" http://127.0.0.1:8080/api/v1/devices/car-001/frame
curl -u "$PIGUARD_HTTP_PLATFORM_USERNAME:$PIGUARD_HTTP_PLATFORM_PASSWORD" http://127.0.0.1:8080/api/v1/snapshots/返回的snapshot_id/content -o downloaded.jpg
```

- multipart 严格包含一个 `file/frame_id/captured_at/type`，拒绝未知、缺失或重复字段。`frame_id` 为 1—64 个 ASCII 字母、数字、下划线或连字符；`captured_at` 使用带时区 RFC3339/RFC3339Nano，返回 UTC；类型为 preview/snapshot/event。
- 文件按实际 JPEG 字节校验，包括尺寸和完整解码；文件名、扩展名和 MIME 声明不决定是否有效。保存原始字节，JPEG ≤2 MiB，总正文 ≤2 MiB+64 KiB，单边 ≤4096、总像素 ≤16,777,216。参数/JPEG 错误返回 400/40001 和字段；资源超限返回 413/40001。
- 新图片返回 201 和稳定 `snapshot_id/device_id/type/captured_at`。同设备、同 `frame_id` 且采集时间、类型、原始字节相同的重试返回 200 和原 ID；冲突返回 409/40903。并发重试只有一份记录/文件，且不重复通知；跨设备相同 frame_id 独立。
- 最新预览接口返回 `snapshot_id/type/captured_at/url` 的 JSON。按 captured_at 选最新，同时间按平台成功提交顺序；迟到旧图仍保存且可按 ID 读取，但不覆盖预览或推送。snapshot/event 不覆盖 preview。
- content 返回 200/image/jpeg 和原始字节。无预览、未知图片或图片文件缺失返回 404/40403；未知设备返回 404/40401。存储/数据库其他错误返回 500/50001，不暴露本地路径。
- 图片文件在 `storage.snapshot_dir`，SQLite 保存不可变身份、摘要、路径与时间元数据。文件持久成功后才提交元数据；提交前失败/取消补偿本次文件，提交后 HTTP 断连保留结果供幂等重试。重启保留最新预览和重试身份，未引用文件不作为有效图片。
- 文件和元数据可读后，新增 snapshot/event 以及成为最新的 preview 发布一次 `frame_update`，data 包含 device_id/snapshot_id/type/captured_at/url，不含 JPEG；timestamp 为平台时间。慢客户端沿用 Hub 的有界队列与关闭策略。
- Windows 图片存储同步 JPEG 普通文件后以不可覆盖方式发布，避免对目录句柄调用不支持的 `Sync()`；Linux 继续同步文件和发布目录。两者均保留失败补偿，硬断电恢复不超出操作系统/文件系统能力；Windows 与 Linux 的软件回归均已覆盖图片存储，硬断电仍需硬件验收。
- 已登记的离线设备及运行中 MQTT 断线均可上传，图片不更新 online、last_seen_at 或配置版本。平台初次启动仍要求 Broker 连接成功。
- 图片持久保留，没有 TTL 或删除接口；拍照与告警引用校验见上节，鉴权接入见上节，容量清理留后续。软件图片的真实 Broker/Docker 链路可以无硬件验收；真实摄像头和树莓派仍待联调。

## 状态约定

- 最新数据按设备 timestamp、同时间下的 seq 排序，旧消息不回退状态；更新时间戳允许设备 seq 归零。设备时钟回拨时，回拨期间的消息不会覆盖已有最新状态。
- `online` 来自 status/遗嘱；遥测不会将离线设备自动标为在线。平台启动或 Broker 断开后，在线状态等待重新确认。
- `samples` 保留采样元数据，`freshness` 区分 fresh/stale/fault/missing。车速、距离、陀螺仪和车道 TTL 为 1 秒，温度为 5 秒；缺失和故障值为 null。
- HTTP 与 WS 使用相同状态视图；WS 在新遥测、状态变化和 TTL 到期时推送。慢客户端会关闭，不阻塞其他连接。
- 历史按每设备最多 1Hz 保存。历史 freshness 表示记录接收时的状态，不随查询时间变成 stale。
- 历史 start/end 接受 RFC3339 时间，包含端点；limit 默认 100，上限 1000。返回窗口内最新 N 条，按时间升序排列。
- 平台重启保留历史和设备配置；最新遥测等待新消息。status 的 config_version 是设备报告，不能代替配置 Ack。

## 原生 Broker 集成检查（本地开发模式）

旧生命周期夹具显式启用 loopback 开发认证模式，会临时启动 Mosquitto 与后端子进程，并覆盖两个 WS 客户端、离线遗嘱、seq 重启、数据过期、平台重启和 Broker 重连。默认认证、设备 ACL 和 Compose 使用前面的 `TestDockerAcceptance`：

```sh
MOSQUITTO_BIN="$(command -v mosquitto)" go test -tags integration ./internal/server -run '^TestMonitoringBrokerLifecycle$' -count=1 -v
```

该检查必须提供可运行的 Mosquitto；没有 Broker 不会将检查跳过后算作通过。

## 设备事件入库确认

`POST /api/v1/devices/{device_id}/events/confirmations` 接收 `{"events":[原始v1事件报文,...]}`，每批 1—64 条、正文不超过 256 KiB。返回 `data.confirmed_event_ids`，仅包括平台已经持久保存且设备、类型、动作、级别、时间、数据和原始图片引用全部一致的记录；不存在或内容冲突的 ID 不确认。此接口不写入事件，也不返回事件内容。

允许平台身份及该设备自己的 Basic 身份。设备不能确认其他设备的事件，也不能访问平台事件列表、WebSocket 或控制接口。边缘端收到确认后才标记 platform_confirmed，缓存清理仍遵守保留期。旧版后端未提供该接口时，边缘端保留未确认数据，不能把 MQTT PUBACK 当作平台入库确认。
