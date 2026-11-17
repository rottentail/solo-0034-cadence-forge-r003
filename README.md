# Cadence Forge · 音视频成片服务

Go HTTP 后端，以真实音视频字节为输入，提供版本化剪辑、字幕时间映射、MP4/WAV加工和可核验交付。没有Web前端。

## 运行

需要 Go 1.24+、Python 3（仅运行有界工作流检查）及带 libx264、AAC、loudnorm、tile、showwavespic 的 FFmpeg/ffprobe。当前本机实际验证 Go 1.27、FFmpeg/ffprobe 9.0.2；Docker构建定义采用Go 1.24和Debian媒体包，镜像运行尚需目标环境独立验证。

```sh
go build -trimpath -o bin/forged ./cmd/forged
./bin/forged
```

默认监听 `127.0.0.1:8734`，数据根 `var/cadence-forge`。可以设置 `FORGE_ADDR`、`FORGE_ROOT`、`FORGE_WORKERS`（1..8，默认2）、`FFMPEG_BIN`、`FFPROBE_BIN`。同一数据根仅允许一个服务进程。服务作为受信任环境中的媒体后端运行，面向公网接入前应部署认证和网络边界。

使用现有媒体工具运行所有真实工作流：

```sh
python3 scripts/workflow_check.py --workflow all
```

检查在临时数据根内生成4秒校准音画与配音，不依赖外部素材或在线服务，完成21条工作流，包括真实强制终止及恢复。`--workflow`可选择运行清单中的某条工作流；所需的前置接收、版本编辑和渲染也会执行。初始化基线不含自动测试套件，testing为deferred；这里提供有界生产工作流核验，不声明测试命令。

## 调用顺序

1. `POST /forge/v1/sources?name=校准片`：请求体为媒体原始字节，返回素材ID、SHA-256与流事实。
2. `POST /forge/v1/recipes`：例如 `{"name":"审片","format":"mp4","width":640,"height":360,"fps":24,"sample_rate":48000,"normalize":false,"target_lufs":-16}`。
3. `POST /forge/v1/sheets`：`{"name":"剪辑","spans":[{"source_id":"素材ID","in_ms":0,"out_ms":1500,"gain_db":0}],"cues":[{"source_id":"素材ID","start_ms":500,"end_ms":1000,"text":"校准"}],"events":[{"id":"爆点","source_id":"素材ID","at_ms":1200}]}`。`events` 是可选的画面事件，供配音锚点对齐。
4. `POST /forge/v1/voices`（可选）：`{"name":"配音","takes":[{"source_id":"配音素材ID","in_ms":0,"out_ms":2000,"gain_db":-2,"anchors":[{"id":"关键词一","event_id":"爆点","cue_ms":400}]}]}`。配音轨与剪辑分别保存、各自版本化。
5. `GET /forge/v1/sheets/{id}/preview?recipe_id={id}&voice_id={id}`：预览当前最新修订的累计区间及WebVTT。片段按半开区间求交；同源重复出现分别映射。带 `voice_id` 时返回配音关系解析与取舍说明。
6. `POST /forge/v1/renders`：提供 `Idempotency-Key`，正文 `{"sheet_id":"剪辑ID","sheet_revision":1,"recipe_id":"配方ID","recipe_revision":1}`；交付配音时再加 `"voice_track_id":"配音轨ID","voice_revision":1`（两者必须成对）。必须显式选择修订，提交把剪辑、配方与配音修订共同冻结为同一份成片编排；预览最新版本不替代提交选择。
7. `GET /forge/v1/renders/{id}`：查看排队、运行、成功、失败或取消及尝试历史；含配音时 `plan.voice` 给出冻结的配音关系说明。
8. `GET /forge/v1/renders/{id}/artifact`：下载检验通过的媒体；`?captions=1`下载字幕，含配音时字幕头部以 `NOTE` 块说明采用的配音关系。`POST .../verify`重新核验；`GET .../bundle`返回媒体、字幕及冻结说明组成的ZIP。

## 配音轨与取舍

配音轨（voice track）是独立版本化的实体：画面剪辑与配音编辑分别保存，正式交付（渲染提交）时两者必须共同指向同一份已确认的成片编排——同一份剪辑修订、配方修订与配音修订被一起冻结，重试与恢复都沿用该冻结快照，后来的编辑不会混入；同源画面重复出现时各片段、各配音段独立映射。

配音段（take）以自然语速播放，锚点（anchor）把段内关键字时刻对准画面事件。画面改稿后，解析器按以下顺序取舍，并把每一次取舍写进 `plan.voice`、渲染记录、交付包清单与字幕 `NOTE` 块，可逐条审阅：

1. 永不整体或局部拉伸声音（不使用 atempo 类处理），也永不把配音钉死在绝对输出时间上。
2. 每个配音段由首个仍被画面覆盖的锚点定位（主锚点）；其余锚点保持源内相对偏移，漂移按毫秒逐条报告（`aligned`/`drifted`）。
3. 锚定位置若与前一段重叠或为负，该段后移到最早可放位置（`shifted`），关键字漂移如实上报——保自然语流与可懂度，牺牲该次对齐。
4. 画面事件被剪掉后，对应锚点标记 `lost`，配音段跟随前驱自然衔接（`flowed`），不静默丢弃也不钉住。
5. 配音超出画面时长时成片延长：MP4 冻结末帧（tpad）、声音继续，延长量写进摘要；不截断配音。
6. 重复源画面下，画面事件取首个覆盖它的片段定位，映射结果公开在 `plan.events`。

预览（带 `voice_id`）在提交前展示同一份解析；提交后解析冻结，失败重试不重新解析。

## 编辑与恢复

`PUT /sheets/{id}`、`PUT /voices/{id}`及`PUT /recipes/{id}`使用十进制修订号 `If-Match: 1`。本服务的该头为应用级修订约定，并非通用HTTP条件请求实现。保存只追加历史，过期写入409。输入中的服务管理ID与修订由服务生成。

`POST /renders/{id}/cancel`撤回排队或运行中的交付；成功确认后该任务不能新提交成片。进程停止是后续动作，并不承诺确认瞬间资源已全部释放。成功成片不可再取消。`POST /renders/{id}/retry`仅接受failed，最多三次尝试，沿用原冻结输入。重启时running变为failed/interrupted，等待显式重试，queued继续排队。

`GET /events?after=0`返回至多200条严格后继事件。序号是持久顺序，时间戳仅用于解释。`GET /state`返回当前一致快照，适合受信任客户端。

## 分析入口

`POST /sources/{id}/loudness`测量源LUFS、真峰值和动态范围，静音或不可测结果使用null并指示silent。`GET /sources/{id}/visual?mode=storyboard`生成四格图；`mode=waveform`生成640×120单声道概览。它们是有成本的同步分析，请求受独立超时约束；波形合并声道可能隐藏反相内容，故事板不等于逐帧审片。

## 支持范围与限制

单素材最多64 MiB、约120秒、3840×2160和8声道。剪辑最多32段、120秒、500条字幕与100个画面事件，段长至少100ms；配音轨最多32段、每段最多8个锚点，段长至少100ms且源必须含声音。输出MP4/H.264/AAC或WAV/16位PCM，采样率44.1或48kHz。MP4要求每段有视频；无声视频生成对应静音。WAV要求每段有声音。视频保持完整画面并留边，统一像素比例与帧率，声音统一立体声。配音与画面声音混音（amix 求和加限幅），不做任何变速处理；归一化为单遍响度滤镜，不保证任何短片都精确达到目标值。

时长基准为毫秒，编码后的帧与采样可能有容差。检验检查流、尺寸、采样率、时长与摘要，不代表内容正确、逐帧同步或主观质量已通过人工审片。禁止容器中的远程协议与非白名单封装，但媒体进程仍需要部署侧隔离。

接收先耐久保留字节再探测登记，失败可能留下未引用对象。成功产物先检验、耐久提升，再提交可见事实；崩溃可能留下未登记产物。当前不自动回收这些内容。`DELETE /sources/{id}`只退休无历史引用的素材记录，保守保留对象；历史剪辑、画面事件、字幕、配音轨和渲染血缘均阻止退休。

状态写入使用同文件系统临时文件、fsync、替换与父目录同步；目录同步失败时服务拒绝后续写入并要求恢复核对。对象及产物与状态分阶段提交，已生成目录不能自行视为交付成功。请备份整个数据根，并在停止唯一服务实例后进行一致复制。

## 容器

```sh
docker build -t cadence-forge .
docker run --rm -p 8734:8734 -v cadence-forge-data:/data cadence-forge
```

运行身份为10001。这里的容器定义不是跨平台运行通过声明；实际验证范围与本机工作流结果见交付记录。
