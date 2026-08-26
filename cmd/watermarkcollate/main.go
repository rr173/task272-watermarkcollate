// 历史手稿水印位置校勘复核台入口。
// 支持 --addr（HTTP 监听地址）、--db（SQLite 路径）、--smoke-test（离线端到端自检）。
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"task272-watermarkcollate/internal/demo"
	"task272-watermarkcollate/internal/httpapi"
	"task272-watermarkcollate/internal/service"
	"task272-watermarkcollate/internal/store"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP 监听地址")
	dbPath := flag.String("db", "watermarkcollate.db", "SQLite 数据库路径")
	smoke := flag.Bool("smoke-test", false, "运行离线端到端自检后退出（不启动长驻服务）")
	flag.Parse()

	if *smoke {
		res, err := demo.RunSmokeTest("")
		if err != nil {
			log.Fatalf("smoke-test 失败: %v", err)
		}
		demo.DumpResult(res)
		fmt.Println("smoke-test 通过：持久化与重启恢复验证成功。")
		os.Exit(0)
	}

	st, err := store.Open(*dbPath)
	if err != nil {
		log.Fatalf("打开数据库: %v", err)
	}
	defer st.Close()

	svc := service.New(st)
	// 启动时执行重启恢复：补齐缺失的水印配对候选。
	rep, err := svc.Recover(context.Background())
	if err != nil {
		log.Printf("重启恢复失败（继续启动）: %v", err)
	} else if rep.PairingsCreated > 0 {
		log.Printf("重启恢复：检查 %d 份手稿，补齐 %d 个水印配对候选", rep.ManuscriptsChecked, rep.PairingsCreated)
	}

	srv := &http.Server{
		Addr:              *addr,
		Handler:           httpapi.New(svc).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	// 优雅退出：SIGINT/SIGTERM 触发关闭。
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("历史手稿水印位置校勘复核台已启动：http://%s （数据库 %s）", *addr, *dbPath)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP 服务异常: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("收到退出信号，正在优雅关闭…")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("关闭异常: %v", err)
	}
	log.Println("已退出。")
}
