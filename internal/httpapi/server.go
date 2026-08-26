package httpapi

import (
	"fmt"
	"net/http"
	"strings"

	"task272-watermarkcollate/internal/service"
)

// Server 是 HTTP 层：只做参数解析、路由与响应，业务全部委托 service。
type Server struct {
	svc *service.Service
	mux *http.ServeMux
}

// New 构造 HTTP Server 并注册全部路由。
func New(svc *service.Service) *Server {
	s := &Server{svc: svc, mux: http.NewServeMux()}
	s.routes()
	return s
}

// Handler 返回底层 http.Handler。
func (s *Server) Handler() http.Handler { return s.mux }

// routes 注册全部 /api 路由与根页面。
func (s *Server) routes() {
	// 手稿批次
	s.mux.HandleFunc("GET /api/manuscripts", s.listManuscripts)
	s.mux.HandleFunc("POST /api/manuscripts", s.createManuscript)
	s.mux.HandleFunc("GET /api/manuscripts/{id}", s.getManuscript)
	s.mux.HandleFunc("PATCH /api/manuscripts/{id}", s.updateManuscriptStatus)
	s.mux.HandleFunc("POST /api/manuscripts/{id}/seal", s.sealManuscript)
	s.mux.HandleFunc("GET /api/manuscripts/{id}/verify", s.verifyManuscript)
	s.mux.HandleFunc("GET /api/manuscripts/{id}/relations", s.listManuscriptRelations)

	// 纸页
	s.mux.HandleFunc("POST /api/manuscripts/{id}/leaves", s.addLeaf)
	s.mux.HandleFunc("GET /api/manuscripts/{id}/leaves", s.listLeaves)
	s.mux.HandleFunc("GET /api/leaves/{id}", s.getLeaf)
	s.mux.HandleFunc("PATCH /api/leaves/{id}", s.updateLeaf)

	// 水印观测
	s.mux.HandleFunc("POST /api/leaves/{id}/watermarks", s.addWatermark)
	s.mux.HandleFunc("GET /api/leaves/{id}/watermarks", s.listWatermarks)
	s.mux.HandleFunc("GET /api/watermarks/{id}", s.getWatermark)
	s.mux.HandleFunc("POST /api/watermarks/{id}/activate", s.activateWatermark)

	// 水印配对
	s.mux.HandleFunc("POST /api/pairings", s.requestPairing)
	s.mux.HandleFunc("GET /api/pairings", s.listPairings)
	s.mux.HandleFunc("GET /api/pairings/{id}", s.getPairing)
	s.mux.HandleFunc("POST /api/pairings/{id}/confirm", s.confirmPairing)

	// 纸张关系
	s.mux.HandleFunc("POST /api/relations", s.createRelation)
	s.mux.HandleFunc("GET /api/relations", s.listRelations)
	s.mux.HandleFunc("GET /api/relations/{id}", s.getRelation)
	s.mux.HandleFunc("POST /api/relations/{id}/adjudicate", s.adjudicateRelation)
	s.mux.HandleFunc("POST /api/relations/{id}/confirm", s.confirmRelation)

	// 重装订候选
	s.mux.HandleFunc("GET /api/candidates", s.rebindCandidates)

	// 校勘版本
	s.mux.HandleFunc("POST /api/versions", s.createVersion)
	s.mux.HandleFunc("GET /api/versions", s.listVersions)
	s.mux.HandleFunc("GET /api/versions/{id}", s.getVersion)
	s.mux.HandleFunc("POST /api/versions/{id}/freeze", s.freezeVersion)
	s.mux.HandleFunc("POST /api/versions/{id}/supersede", s.supersedeVersion)

	// 统计与自检
	s.mux.HandleFunc("GET /api/stats", s.stats)
	s.mux.HandleFunc("POST /api/selfcheck", s.selfcheck)

	// 根页面：最小复核工作台。
	s.mux.HandleFunc("GET /", s.indexPage)
}

// pathID 从路径参数读取 ID（Go 1.22 路由路径参数）。
func pathID(r *http.Request, key string) string {
	v := r.PathValue(key)
	if v == "" {
		return ""
	}
	return strings.TrimSpace(v)
}

// indexPage 最小 Web 页面：展示 API 入口与使用说明。
func (s *Server) indexPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		writeErr(w, fmt.Errorf("NOT_FOUND: 路径 %s 不存在", r.URL.Path))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, indexHTML)
}

const indexHTML = `<!DOCTYPE html>
<html lang="zh">
<head>
<meta charset="utf-8">
<title>历史手稿水印位置校勘复核台</title>
<style>
body{font-family:-apple-system,"PingFang SC",sans-serif;max-width:880px;margin:32px auto;padding:0 20px;color:#222}
h1{font-size:22px}code{background:#f2f2f2;padding:2px 6px;border-radius:4px;font-size:13px}
li{margin:6px 0}
</style>
</head>
<body>
<h1>历史手稿水印位置校勘复核台</h1>
<p>面向纸本文献研究者：导入纸页观测与水印半片，自动配对水印、校验链线方向与折页连续性，
裁决重装订候选并发布不可变校勘版本。全部数据持久化于 SQLite。</p>
<h2>核心 API（前缀 /api）</h2>
<ul>
<li><code>POST /api/manuscripts</code> 创建手稿批次</li>
<li><code>POST /api/manuscripts/{id}/leaves</code> 导入纸页观测</li>
<li><code>POST /api/leaves/{id}/watermarks</code> 登记水印半片</li>
<li><code>POST /api/pairings</code> 请求水印半片配对</li>
<li><code>POST /api/relations</code> 建立相邻纸页关系（自动证据计算）</li>
<li><code>GET /api/manuscripts/{id}/verify</code> 折页连续性校验</li>
<li><code>GET /api/candidates</code> 重装订候选汇总</li>
<li><code>POST /api/versions/{id}/freeze</code> 冻结校勘版本</li>
</ul>
<p>完整 API 清单见 README.md；离线自检：<code>--smoke-test</code>。</p>
</body>
</html>`
