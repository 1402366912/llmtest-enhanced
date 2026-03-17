package api

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许所有来源
	},
}

// handleWebSocket 处理WebSocket连接
func (s *Server) handleWebSocket(c *gin.Context) {
	runID := c.Query("run_id")
	if runID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "run_id 是必填参数",
		})
		return
	}

	run := s.GetRun(runID)
	if run == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "未找到指定的测试运行",
		})
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("WebSocket升级失败: %v", err)
		return
	}

	// 注册客户端到指定的 Run
	run.WSMu.Lock()
	run.WSClients[conn] = true
	run.WSMu.Unlock()

	log.Printf("WebSocket客户端已连接到 run[%s]，当前连接数: %d", runID, len(run.WSClients))

	// 发送当前测试状态
	conn.WriteJSON(map[string]interface{}{
		"type":       "status",
		"run_id":     run.ID,
		"running":    run.Running,
		"machine_id": run.MachineID,
		"backend_id": run.BackendID,
		"model_name": run.ModelName,
	})

	// 如果有最后一条消息（可能是错误或完成消息），也发送给新连接的客户端
	run.LastMessageMu.RLock()
	lastMsg := run.LastMessage
	run.LastMessageMu.RUnlock()
	if lastMsg != nil {
		conn.WriteJSON(lastMsg)
	}

	// 监听客户端断开
	go func() {
		defer func() {
			run.WSMu.Lock()
			delete(run.WSClients, conn)
			run.WSMu.Unlock()
			conn.Close()
			log.Printf("WebSocket客户端已断开 run[%s]，当前连接数: %d", runID, len(run.WSClients))
		}()

		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				break
			}
		}
	}()
}
