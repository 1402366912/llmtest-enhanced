package api

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/lemonlinger/llm-test/config"
	"github.com/lemonlinger/llm-test/engine"
)

// Run 表示一个独立的测试运行实例
type Run struct {
	ID            string
	MachineID     string
	BackendID     string
	ModelName     string
	Running       bool
	Cancel        context.CancelFunc
	Ctx           context.Context
	ProgressChan  chan *TestProgress
	LatestResults *TestResultStore
	WSClients     map[*websocket.Conn]bool
	WSMu          sync.Mutex
	StartTime     time.Time
	ResultsMu     sync.RWMutex
	// 最后一条消息（用于 WebSocket 连接时补发）
	LastMessage   *TestProgress
	LastMessageMu sync.RWMutex
}

// Server API服务器
type Server struct {
	router     *gin.Engine
	config     *config.Config
	configPath string
	mu         sync.RWMutex
	// 多 Run 管理
	runs   map[string]*Run
	runsMu sync.RWMutex
}

// TestProgress 测试进度
type TestProgress struct {
	Type            string  `json:"type"`             // "progress", "result", "error", "complete"
	ModelName       string  `json:"model_name"`
	Concurrency     int     `json:"concurrency"`
	ContextTokens   int     `json:"context_tokens"`
	CurrentRequests int     `json:"current_requests"`
	TotalRequests   int     `json:"total_requests"`
	SuccessRequests int     `json:"success_requests"`
	FailedRequests  int     `json:"failed_requests"`
	AvgTTFTMs       float64 `json:"avg_ttft_ms"`      // 平均首Token延迟(毫秒)
	PrefillTPS      float64 `json:"prefill_tps"`      // 预填充速度 (输入tokens/首token延迟)
	RPS             float64 `json:"rps"`
	TPS             float64 `json:"tps"`              // 总体TPS
	DecodeTPSAvg    float64 `json:"decode_tps_avg"`   // 平均生成速度
	Message         string  `json:"message,omitempty"`
}

// NewServer 创建API服务器
func NewServer(configPath string) (*Server, error) {
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return nil, err
	}

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()

	// 配置CORS
	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		AllowCredentials: true,
	}))

	s := &Server{
		router:     router,
		config:     cfg,
		configPath: configPath,
		runs:       make(map[string]*Run),
	}

	s.setupRoutes()

	return s, nil
}

// NewRun 创建一个新的测试运行实例
func (s *Server) NewRun(machineID, backendID, modelName string) *Run {
	ctx, cancel := context.WithCancel(context.Background())
	run := &Run{
		ID:           uuid.New().String(),
		MachineID:    machineID,
		BackendID:    backendID,
		ModelName:    modelName,
		Running:      false,
		Cancel:       cancel,
		Ctx:          ctx,
		ProgressChan: make(chan *TestProgress, 100),
		WSClients:    make(map[*websocket.Conn]bool),
		StartTime:    time.Now(),
	}

	s.runsMu.Lock()
	s.runs[run.ID] = run
	s.runsMu.Unlock()

	// 启动该 Run 的进度广播协程
	go s.broadcastRunProgress(run)

	return run
}

// GetRun 获取指定的运行实例
func (s *Server) GetRun(runID string) *Run {
	s.runsMu.RLock()
	defer s.runsMu.RUnlock()
	return s.runs[runID]
}

// RemoveRun 移除指定的运行实例
func (s *Server) RemoveRun(runID string) {
	s.runsMu.Lock()
	defer s.runsMu.Unlock()
	if run, ok := s.runs[runID]; ok {
		// 关闭进度通道
		close(run.ProgressChan)
		delete(s.runs, runID)
	}
}

// broadcastRunProgress 广播指定 Run 的测试进度
func (s *Server) broadcastRunProgress(run *Run) {
	for progress := range run.ProgressChan {
		// 保存最后一条消息（用于新连接的客户端）
		run.LastMessageMu.Lock()
		run.LastMessage = progress
		run.LastMessageMu.Unlock()

		run.WSMu.Lock()
		for client := range run.WSClients {
			err := client.WriteJSON(progress)
			if err != nil {
				log.Printf("WebSocket发送失败[run=%s]: %v", run.ID, err)
				client.Close()
				delete(run.WSClients, client)
			}
		}
		run.WSMu.Unlock()
	}
}

// TestResultStore 测试结果存储（从 test.go 移动到这里以避免循环依赖）
type TestResultStore struct {
	Results   map[string]*engine.TestResult `json:"results"`
	StartTime time.Time                     `json:"start_time"`
	EndTime   time.Time                     `json:"end_time"`
	Duration  string                        `json:"duration"`
}

// setupRoutes 设置路由
func (s *Server) setupRoutes() {
	api := s.router.Group("/api")
	{
		// 配置相关
		api.GET("/config", s.getConfig)
		api.PUT("/config", s.updateConfig)
		api.POST("/config/reload", s.reloadConfig)

		// 模型相关
		api.GET("/models", s.getModels)
		api.POST("/models", s.addModel)
		api.PUT("/models/:name", s.updateModel)
		api.DELETE("/models/:name", s.deleteModel)
		api.PUT("/models/:name/toggle", s.toggleModel)

		// 机器管理相关
		api.GET("/machines", s.getMachines)
		api.POST("/machines", s.createMachine)
		api.PUT("/machines/:id", s.updateMachine)
		api.DELETE("/machines/:id", s.deleteMachine)
		api.POST("/machines/:id/select", s.selectMachine)
		api.GET("/machines/:id/config", s.getMachineConfig)
		api.PUT("/machines/:id/config", s.updateMachineConfig)
		api.GET("/machines/current", s.getCurrentMachine)

		// 后端管理相关
		api.GET("/machines/:id/backends", s.getBackends)
		api.POST("/machines/:id/backends", s.createBackend)
		api.PUT("/machines/:id/backends/:backend_id", s.updateBackend)
		api.DELETE("/machines/:id/backends/:backend_id", s.deleteBackend)
		api.POST("/machines/:id/backends/:backend_id/select", s.selectBackend)
		api.GET("/machines/:id/backends/:backend_id/config", s.getBackendConfig)
		api.PUT("/machines/:id/backends/:backend_id/config", s.updateBackendConfig)
		api.GET("/machines/:id/backends/current", s.getCurrentBackend)

		// 历史记录相关
		api.GET("/history", s.getHistory)
		api.GET("/history/:id", s.getHistoryDetail)
		api.DELETE("/history/:id", s.deleteHistory)
		api.GET("/history/:id/export", s.exportHistory)

		// 天梯图数据
		api.GET("/leaderboard", s.getLeaderboard)

		// 数据分析
		api.GET("/analytics", s.getAnalyticsData)
		api.GET("/analytics/models", s.getAvailableModels)

		// 数据导入导出
		api.GET("/export", s.exportData)
		api.POST("/import", s.importData)

		// 数据集预览
		api.GET("/dataset/preview", s.previewDataset)

		// 测试相关
		api.POST("/test/start", s.startTest)
		api.POST("/test/stop", s.stopTest)
		api.GET("/test/status", s.getTestStatus)
		api.GET("/test/results", s.getTestResults)
		api.POST("/test/connection", s.testConnection)

		// WebSocket
		api.GET("/ws", s.handleWebSocket)
	}

	// 设置嵌入的静态文件
	s.setupStaticFiles()
}

// Run 启动服务器
func (s *Server) Run(addr string) error {
	return s.router.Run(addr)
}

// getConfig 获取配置
func (s *Server) getConfig(c *gin.Context) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    s.config,
	})
}

// updateConfig 更新配置
func (s *Server) updateConfig(c *gin.Context) {
	var newConfig config.Config
	if err := c.ShouldBindJSON(&newConfig); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	// 根据起始和结束值重新生成上下文长度梯度
	// 只要设置了 start 和 end，就重新生成（覆盖旧值）
	if newConfig.Test.ContextTokenStart > 0 && newConfig.Test.ContextTokenEnd > 0 {
		newConfig.Test.ContextTokenLevels = config.GenerateContextLevels(newConfig.Test.ContextTokenStart, newConfig.Test.ContextTokenEnd)
	}

	s.mu.Lock()
	s.config = &newConfig
	s.mu.Unlock()

	// 保存到文件
	if err := s.saveConfig(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "配置已更新",
	})
}

// reloadConfig 重新加载配置
func (s *Server) reloadConfig(c *gin.Context) {
	cfg, err := config.LoadConfig(s.configPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	s.mu.Lock()
	s.config = cfg
	s.mu.Unlock()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "配置已重新加载",
	})
}

// getModels 获取模型列表
func (s *Server) getModels(c *gin.Context) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    s.config.Models,
	})
}

// addModel 添加模型
func (s *Server) addModel(c *gin.Context) {
	// 读取原始请求体用于调试
	bodyBytes, _ := io.ReadAll(c.Request.Body)
	c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	log.Printf("收到添加模型请求，原始数据: %s", string(bodyBytes))

	var model config.ModelConfig
	if err := c.ShouldBindJSON(&model); err != nil {
		log.Printf("JSON 绑定失败: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	log.Printf("绑定后的模型数据: Name=%s, Type=%s, APIKey=%s, BaseURL=%s, Params=%+v",
		model.Name, model.Type, model.APIKey, model.BaseURL, model.Params)

	// 验证必填字段
	if model.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "模型名称不能为空",
		})
		return
	}

	// 确保 Params 不为 nil
	if model.Params == nil {
		model.Params = make(map[string]interface{})
	}

	// 检查是否已存在同名模型
	s.mu.Lock()
	for _, m := range s.config.Models {
		if m.Name == model.Name {
			s.mu.Unlock()
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "模型名称已存在",
			})
			return
		}
	}
	s.config.Models = append(s.config.Models, model)
	s.mu.Unlock()

	log.Printf("准备保存模型，最终数据: Name=%s, Type=%s, APIKey=%s, BaseURL=%s",
		model.Name, model.Type, model.APIKey, model.BaseURL)

	if err := s.saveConfig(); err != nil {
		log.Printf("保存配置失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	log.Printf("模型已成功保存到配置文件")
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "模型已添加",
	})
}

// updateModel 更新模型
func (s *Server) updateModel(c *gin.Context) {
	name := c.Param("name")
	
	// 读取原始请求体用于调试
	bodyBytes, _ := io.ReadAll(c.Request.Body)
	c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	log.Printf("收到更新模型请求 [%s]，原始数据: %s", name, string(bodyBytes))

	var model config.ModelConfig
	if err := c.ShouldBindJSON(&model); err != nil {
		log.Printf("JSON 绑定失败: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	log.Printf("绑定后的模型数据: Name=%s, Type=%s, APIKey=%s, BaseURL=%s, Params=%+v",
		model.Name, model.Type, model.APIKey, model.BaseURL, model.Params)

	// 确保模型名称匹配
	model.Name = name

	// 确保 Params 不为 nil
	if model.Params == nil {
		model.Params = make(map[string]interface{})
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	found := false
	for i, m := range s.config.Models {
		if m.Name == name {
			// 完全替换模型配置，确保所有字段都被更新
			log.Printf("找到模型 [%s]，准备更新", name)
			s.config.Models[i] = model
			found = true
			break
		}
	}

	if !found {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "模型不存在",
		})
		return
	}

	log.Printf("准备保存更新后的模型，最终数据: Name=%s, Type=%s, APIKey=%s, BaseURL=%s",
		model.Name, model.Type, model.APIKey, model.BaseURL)

	if err := s.saveConfig(); err != nil {
		log.Printf("保存配置失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	log.Printf("模型已成功更新到配置文件")
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "模型已更新",
	})
}

// deleteModel 删除模型
func (s *Server) deleteModel(c *gin.Context) {
	name := c.Param("name")

	s.mu.Lock()
	defer s.mu.Unlock()

	for i, m := range s.config.Models {
		if m.Name == name {
			s.config.Models = append(s.config.Models[:i], s.config.Models[i+1:]...)
			if err := s.saveConfig(); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"success": false,
					"error":   err.Error(),
				})
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"message": "模型已删除",
			})
			return
		}
	}

	c.JSON(http.StatusNotFound, gin.H{
		"success": false,
		"error":   "模型不存在",
	})
}

// toggleModel 切换模型启用状态
func (s *Server) toggleModel(c *gin.Context) {
	name := c.Param("name")

	s.mu.Lock()
	defer s.mu.Unlock()

	for i, m := range s.config.Models {
		if m.Name == name {
			s.config.Models[i].Skip = !s.config.Models[i].Skip
			if err := s.saveConfig(); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"success": false,
					"error":   err.Error(),
				})
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"message": "模型状态已切换",
				"data": gin.H{
					"skip": s.config.Models[i].Skip,
				},
			})
			return
		}
	}

	c.JSON(http.StatusNotFound, gin.H{
		"success": false,
		"error":   "模型不存在",
	})
}

// getTestStatus 获取测试状态
func (s *Server) getTestStatus(c *gin.Context) {
	runID := c.Query("run_id")
	
	if runID != "" {
		// 查询指定 run 的状态
		run := s.GetRun(runID)
		if run == nil {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   "未找到指定的测试运行",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data": gin.H{
				"run_id":     run.ID,
				"running":    run.Running,
				"machine_id": run.MachineID,
				"backend_id": run.BackendID,
				"model_name": run.ModelName,
				"start_time": run.StartTime,
			},
		})
		return
	}

	// 返回所有活跃的 run 状态
	s.runsMu.RLock()
	defer s.runsMu.RUnlock()

	activeRuns := make([]gin.H, 0)
	for _, run := range s.runs {
		activeRuns = append(activeRuns, gin.H{
			"run_id":     run.ID,
			"running":    run.Running,
			"machine_id": run.MachineID,
			"backend_id": run.BackendID,
			"model_name": run.ModelName,
			"start_time": run.StartTime,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"runs": activeRuns,
		},
	})
}

// DatasetPreview 数据集预览结果
type DatasetPreview struct {
	Path       string   `json:"path"`
	Exists     bool     `json:"exists"`
	TotalCount int      `json:"total_count"`
	Samples    []string `json:"samples"`
	Error      string   `json:"error,omitempty"`
}

// previewDataset 预览数据集
func (s *Server) previewDataset(c *gin.Context) {
	s.mu.RLock()
	datasetPath := s.config.Prompt.DatasetPath
	s.mu.RUnlock()

	preview := DatasetPreview{
		Path:    datasetPath,
		Samples: []string{},
	}

	if datasetPath == "" {
		preview.Error = "未配置数据集路径"
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    preview,
		})
		return
	}

	// 检查文件是否存在
	file, err := os.Open(datasetPath)
	if err != nil {
		preview.Exists = false
		preview.Error = "无法打开文件: " + err.Error()
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    preview,
		})
		return
	}
	defer file.Close()

	preview.Exists = true

	// 读取数据集
	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	var prompts []string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			prompts = append(prompts, line)
		}
	}

	if err := scanner.Err(); err != nil {
		preview.Error = "读取文件错误: " + err.Error()
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    preview,
		})
		return
	}

	preview.TotalCount = len(prompts)

	// 取前10条作为样本预览
	maxSamples := 10
	if len(prompts) < maxSamples {
		maxSamples = len(prompts)
	}
	for i := 0; i < maxSamples; i++ {
		// 截断过长的提示词
		sample := prompts[i]
		if len(sample) > 200 {
			sample = sample[:200] + "..."
		}
		preview.Samples = append(preview.Samples, sample)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    preview,
	})
}
