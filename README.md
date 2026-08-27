# 野镜影像发布工作台

野镜影像发布工作台面向自然保护区科研团队，用一个可追溯的本地业务闭环管理红外相机影像。整理员建立批次并批量预检、登记带 SHA-256 摘要的影像，两个席位按独立待办连续盲标，系统运行可复算的确定性质检；专家按证据级队列预校验仲裁决定或发起有基线的整改轮次，发布负责人随后核对清单字段差异、冻结规范化清单、签发或导入 Ed25519 发布凭据并分层验证完整性。

服务不依赖外部数据库或 Node 构建链。事件账本、原子投影快照、幂等结果、签名密钥和按摘要寻址的影像载荷都保存在本地数据目录中。事件账本使用单调序号和 SHA-256 前序摘要链，服务启动时会校验并恢复投影。

## 构建

```sh
go build ./cmd/wildframe
```

## 运行

```sh
go run ./cmd/wildframe -addr=127.0.0.1:19081 -data=./wildframe-data
```

浏览器访问 `http://127.0.0.1:19081/`。默认地址也是 `127.0.0.1:19081`，不会绑定 `0.0.0.0`。未传 `-addr` 时可以通过 `PORT` 提供端口号，服务会绑定 `127.0.0.1:<PORT>`：

```sh
PORT=19123 go run ./cmd/wildframe
```

为保护本地影像，显式 `-addr` 也必须使用回环地址。单个上传载荷最多 12 MiB，仅接受 JPEG、PNG 或 WebP。

## 自检

自检会创建临时数据目录，启动真实回环 HTTP 服务，通过与浏览器相同的 API 完成建档、上传、双席位分歧标注、质检、专家仲裁、复核、冻结、签发和验证，并在 15 秒内自动退出：

```sh
go run ./cmd/wildframe -addr=127.0.0.1:19081 -selfcheck
```

## 测试

```sh
go test ./...
```

测试覆盖质检规则与凭据签验、账本重放、盲标隔离、幂等复用、过期版本拒绝、完整发布流程、工作台资源和 HTTP 输入边界。

## 主要 API

- `POST /api/v1/collections`：建立批次。
- `POST /api/v1/collections/{collectionID}/evidence`：以 `multipart/form-data` 登记和上传单幅影像。
- `POST /api/v1/collections/{collectionID}/evidence/batch`：批量上传影像，逐项返回登记、重复、校验、版本或可重试存储结果。
- `POST /api/v1/collections/{collectionID}/annotations`：提交席位独立标注或整改修订。
- `GET /api/v1/collections/{collectionID}`：通过 `seat`、`task`、`sort` 及质检筛选参数读取盲标进度、待办和队列。
- `POST /api/v1/collections/{collectionID}/quality-runs`：按批次规则版本确定性重新质检并记录输入、结果摘要。
- `POST /api/v1/collections/{collectionID}/adjudications/preview` 与 `POST .../adjudications`：预校验并提交采纳席位、新结论或定向整改。
- `POST /api/v1/collections/{collectionID}/review`：专家复核通过。
- `GET /api/v1/collections/{collectionID}/manifest` 与 `POST .../freeze`：生成统计、字段差异和版本绑定预览，并按摘要冻结清单。
- `POST /api/v1/collections/{collectionID}/credential`：签发发布凭据。
- `POST /api/v1/credentials/verify`：独立验证凭据签名，再报告本地批次和冻结摘要绑定状态。
- `GET /api/v1/collections/{collectionID}/audit`：按操作者、动作、时间和状态迁移筛选，以 `after` sequence 游标分页并返回完整链状态耗时。
