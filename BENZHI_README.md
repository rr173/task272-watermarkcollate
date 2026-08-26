# BENZHI 评测说明

基于 Go 实现的历史手稿水印位置校勘复核后端服务，一款后端服务，完成水印半片配对、链线方向与折页连续性校验、重装订裁决与不可变校勘版本冻结。

## 启动

```bash
CGO_ENABLED=0 GOTOOLCHAIN=local go run ./cmd/watermarkcollate --addr :8080 --db watermarkcollate.db
```

## 自检（不启动长驻服务）

```bash
go run ./cmd/watermarkcollate --smoke-test
```

`--smoke-test` 会真实创建手稿与纸页、登记互补水印半片、请求配对、建立相邻关系、校验折页连续性、冻结校勘版本，关闭并重新打开数据库验证持久化与重启恢复，最后以 0 退出码结束。

## 构建门禁

```bash
CGO_ENABLED=0 GOTOOLCHAIN=local go build ./...
CGO_ENABLED=0 GOTOOLCHAIN=local go vet   ./...
CGO_ENABLED=0 GOTOOLCHAIN=local go test  ./...
go run ./cmd/watermarkcollate --smoke-test
```

## HTTP API（前缀 /api）

手稿：`POST /api/manuscripts`、`GET /api/manuscripts`、`GET /api/manuscripts/{id}`、`PATCH /api/manuscripts/{id}`、`POST /api/manuscripts/{id}/seal`、`GET /api/manuscripts/{id}/verify`
纸页：`POST /api/manuscripts/{id}/leaves`、`GET /api/manuscripts/{id}/leaves`、`GET /api/leaves/{id}`、`PATCH /api/leaves/{id}`
水印：`POST /api/leaves/{id}/watermarks`、`GET /api/leaves/{id}/watermarks`、`GET /api/watermarks/{id}`、`POST /api/watermarks/{id}/activate`
配对：`POST /api/pairings`、`GET /api/pairings`、`GET /api/pairings/{id}`、`POST /api/pairings/{id}/confirm`
关系：`POST /api/relations`、`GET /api/relations`、`GET /api/relations/{id}`、`POST /api/relations/{id}/adjudicate`、`POST /api/relations/{id}/confirm`
候选：`GET /api/candidates`
版本：`POST /api/versions`、`GET /api/versions`、`GET /api/versions/{id}`、`POST /api/versions/{id}/freeze`、`POST /api/versions/{id}/supersede`
统计与自检：`GET /api/stats`、`POST /api/selfcheck`

## 持久化

SQLite（modernc.org/sqlite，CGO 无关）。建表：manuscripts、leaves、watermark_observations、watermark_pairings、leaf_relations、collation_versions。纸页/半片/配对/关系/版本唯一键幂等；冻结校勘版本的快照不可改写。
