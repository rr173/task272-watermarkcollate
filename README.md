# 历史手稿水印位置校勘复核台（task272-watermarkcollate）

面向纸本文献研究者（版本学、物质书志学方向）的纸张结构复核服务：
导入手稿纸页观测与水印半片证据，自动执行**水印半片配对**、**链线方向校验**与**折页连续性验证**，
裁决相邻纸页是**同折页**还是**重装订候选**，支持研究者调整观察置信度、人工裁决并发布**不可变校勘版本**。

不是文档 CMS、文件管理或相册：页面不编辑/分享手稿内容，只复核纸页之间的物理制造与装订关系。

## 业务闭环

```
导入手稿 → 导入纸页观测（页码/折页/开面/链线/置信度）→ 登记水印半片 → 激活观测
→ 水印半片配对（同模具对 + 位置互补）→ 建立相邻关系（自动聚合链线/折页/水印证据）
→ 折页连续性校验（发现断裂点）→ 裁决（同折页 / 重装订 / 冲突）→ 发布并冻结校勘版本
```

## 状态机

| 实体 | 状态机 |
| --- | --- |
| 手稿批次 | organizing → collating → adjudicating → published → sealed（sealed 终态） |
| 纸页观测 | pending → valid / damaged → excluded（excluded 终态） |
| 水印观测 | pending → valid / excluded |
| 水印配对 | candidate → matched / unmatched → confirmed / rejected |
| 纸张关系 | candidate → same_fold / rebound / conflict → confirmed |
| 校勘版本 | draft → shared → frozen → superseded |

## 核心算法

- **水印配对**（`internal/watermark`）：模具对一致 0.5 + 左右半片对称 0.3 + 置信度 0.2 → 匹配度评分，
  ≥0.80 判 matched，≥0.60 候选，否则 unmatched。
- **链线校验**（`internal/chainline`）：同向（≤15°）或折叠轴对称（镜像 ≤15°）视为一致。
- **折页连续性**（`internal/quire`）：页码连续且折页号连续才视为同折页连续，断裂点列为重装订候选。
- **重装订裁决**（`internal/adjudicate`）：0.4×水印 + 0.3×链线 + 0.3×折页 → ≥0.75 同折页、≤0.45 重装订、其余冲突待人工。

## 构建与测试

```bash
CGO_ENABLED=0 GOTOOLCHAIN=local go build ./...
CGO_ENABLED=0 GOTOOLCHAIN=local go vet   ./...
CGO_ENABLED=0 GOTOOLCHAIN=local go test  ./...
go run ./cmd/watermarkcollate --smoke-test
```

## 运行

```bash
CGO_ENABLED=0 GOTOOLCHAIN=local go run ./cmd/watermarkcollate --addr :8080 --db watermarkcollate.db
# 浏览器打开 http://localhost:8080/ 查看说明页
```

## 持久化

SQLite（modernc.org/sqlite，纯 Go，CGO 无关）六表：
`manuscripts`、`leaves`、`watermark_observations`、`watermark_pairings`、`leaf_relations`、`collation_versions`。
唯一键：纸页 (manuscript_id, page_no)、半片 (leaf_id, half_id)、配对 (a,b)、关系 (left,right)、版本 (manuscript_id, version_no)。
服务启动执行 `Recover` 补齐缺失配对候选；关闭重开数据库全部状态恢复。

## 并发与错误边界

- 所有写入经事务；手稿/纸页/配对/关系使用乐观版本锁，版本冲突返回 `VERSION_MISMATCH`。
- 拒绝：页码重复、链线方向越界、水印坐标越界、冻结版本修改、封存手稿写入、非法状态迁移。
- 领域错误统一 `model.DomainError`，HTTP 层按 `Code` 映射 404/409/422。

## API 摘要

前缀 `/api`，完整清单见 `BENZHI_README.md`。核心入口：
`POST /api/manuscripts`、`POST /api/manuscripts/{id}/leaves`、`POST /api/leaves/{id}/watermarks`、
`POST /api/pairings`、`POST /api/relations`、`GET /api/manuscripts/{id}/verify`、
`GET /api/candidates`、`POST /api/versions/{id}/freeze`。
