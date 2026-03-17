package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/lemonlinger/llm-test/api"
)

func main() {
	configFile := flag.String("config", "config.yaml", "配置文件路径")
	configFile2 := flag.String("c", "", "配置文件路径 (简写)")
	addr := flag.String("addr", ":8088", "服务器监听地址")
	addr2 := flag.String("p", "", "端口 (简写，如 8088)")
	flag.Parse()

	// 处理简写参数
	if *configFile2 != "" {
		*configFile = *configFile2
	}
	if *addr2 != "" {
		*addr = ":" + *addr2
	}

	fmt.Println()
	fmt.Println("  ╔═══════════════════════════════════════╗")
	fmt.Println("  ║     LLM-Test 性能测试平台             ║")
	fmt.Println("  ╚═══════════════════════════════════════╝")
	fmt.Println()
	fmt.Printf("  配置文件: %s\n", *configFile)
	fmt.Printf("  监听地址: %s\n", *addr)
	fmt.Println()

	server, err := api.NewServer(*configFile)
	if err != nil {
		log.Fatalf("❌ 创建服务器失败: %v", err)
	}

	// 优雅退出
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		fmt.Println("\n\n  正在关闭服务器...")
		os.Exit(0)
	}()

	fmt.Println("  ✓ 服务器已启动")
	fmt.Printf("  ✓ 访问地址: http://localhost%s\n", *addr)
	fmt.Println()
	fmt.Println("  按 Ctrl+C 停止服务器")
	fmt.Println()

	if err := server.Run(*addr); err != nil {
		log.Fatalf("❌ 服务器运行失败: %v", err)
	}
}
