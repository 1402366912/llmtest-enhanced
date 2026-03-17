package api

import (
	"context"
	"log"
	"net/http"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lemonlinger/llm-test/config"
	"github.com/lemonlinger/llm-test/engine"
	"github.com/lemonlinger/llm-test/model"
)

// StartTestRequest 启动测试请求
type StartTestRequest struct {
	MachineID string `json:"machine_id"`
	BackendID string `json:"backend_id"`
	ModelName string `json:"model_name"`
}

// StartTestResponse 启动测试响应
type StartTestResponse struct {
	RunID string `json:"run_id"`
}

// StopTestRequest 停止测试请求
type StopTestRequest struct {
	RunID string `json:"run_id"`
}

// startTest 启动测试
func (s *Server) startTest(c *gin.Context) {
	var req StartTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "请求参数错误: " + err.Error(),
		})
		return
	}

	// 验证必填字段
	if req.MachineID == "" || req.BackendID == "" || req.ModelName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "machine_id, backend_id, model_name 都是必填项",
		})
		return
	}

	// 创建新的 Run
	run := s.NewRun(req.MachineID, req.BackendID, req.ModelName)
	run.Running = true

	// 异步执行测试
	go s.runTestForRun(run)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "测试已启动",
		"data": StartTestResponse{
			RunID: run.ID,
		},
	})
}

// stopTest 停止测试
func (s *Server) stopTest(c *gin.Context) {
	var req StopTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "请求参数错误: " + err.Error(),
		})
		return
	}

	if req.RunID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "run_id 是必填项",
		})
		return
	}

	run := s.GetRun(req.RunID)
	if run == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "未找到指定的测试运行",
		})
		return
	}

	if !run.Running {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "该测试已经停止",
		})
		return
	}

	if run.Cancel != nil {
		run.Cancel()
		// 通知前端测试已停止
		run.ProgressChan <- &TestProgress{
			Type:    "stopped",
			Message: "测试已停止",
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "正在停止测试...",
	})
}

// ConnectionTestRequest 连接测试请求
type ConnectionTestRequest struct {
	ModelName string `json:"model_name"`
}

// ConnectionTestResult 连接测试结果
type ConnectionTestResult struct {
	Success     bool    `json:"success"`
	ModelName   string  `json:"model_name"`
	Message     string  `json:"message"`
	LatencyMs   float64 `json:"latency_ms"`
	TokenCount  int     `json:"token_count,omitempty"`
	Response    string  `json:"response,omitempty"`
}

// testConnection 测试模型连接
func (s *Server) testConnection(c *gin.Context) {
	var req ConnectionTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "请求参数错误: " + err.Error(),
		})
		return
	}

	s.mu.RLock()
	cfg := s.config
	s.mu.RUnlock()

	// 查找指定模型
	var targetModel *config.ModelConfig
	for i := range cfg.Models {
		if cfg.Models[i].Name == req.ModelName {
			targetModel = &cfg.Models[i]
			break
		}
	}

	if targetModel == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "模型不存在: " + req.ModelName,
		})
		return
	}

	// 初始化模型
	models, err := model.InitializeModels([]config.ModelConfig{*targetModel}, cfg.Proxies)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data": ConnectionTestResult{
				Success:   false,
				ModelName: req.ModelName,
				Message:   "模型初始化失败: " + err.Error(),
			},
		})
		return
	}

	if len(models) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data": ConnectionTestResult{
				Success:   false,
				ModelName: req.ModelName,
				Message:   "模型初始化失败：无可用模型",
			},
		})
		return
	}

	testModel := models[0]

	// 发送测试请求
	startTime := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := testModel.GenerateResponse(ctx, "你是一个测试助手", "你好，请简单回复一句话测试连接", false)
	latency := time.Since(startTime).Seconds() * 1000

	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data": ConnectionTestResult{
				Success:   false,
				ModelName: req.ModelName,
				Message:   "请求失败: " + err.Error(),
				LatencyMs: latency,
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": ConnectionTestResult{
			Success:    true,
			ModelName:  req.ModelName,
			Message:    "连接成功",
			LatencyMs:  latency,
			TokenCount: resp.OutputTokens,
			Response:   resp.Content,
		},
	})
}

// getTestResults 获取测试结果
func (s *Server) getTestResults(c *gin.Context) {
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

	run.ResultsMu.RLock()
	defer run.ResultsMu.RUnlock()

	if run.LatestResults == nil {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    nil,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    run.LatestResults,
	})
}

// runTestForRun 为指定的 Run 执行测试
func (s *Server) runTestForRun(run *Run) {
	defer func() {
		run.Running = false
	}()

	startTime := time.Now()

	// 发送开始消息
	run.ProgressChan <- &TestProgress{
		Type:    "start",
		Message: "测试开始",
	}

	// 加载该 backend 的配置
	configPath := filepath.Join("data", run.MachineID, run.BackendID, "config.yaml")
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		run.ProgressChan <- &TestProgress{
			Type:    "error",
			Message: "加载配置失败: " + err.Error(),
		}
		return
	}

	// 查找指定的模型配置
	var targetModelConfig *config.ModelConfig
	for i := range cfg.Models {
		if cfg.Models[i].Name == run.ModelName {
			targetModelConfig = &cfg.Models[i]
			break
		}
	}

	if targetModelConfig == nil {
		run.ProgressChan <- &TestProgress{
			Type:    "error",
			Message: "未找到指定的模型: " + run.ModelName,
		}
		return
	}

	// 只初始化指定的模型
	models, err := model.InitializeModels([]config.ModelConfig{*targetModelConfig}, cfg.Proxies)
	if err != nil {
		run.ProgressChan <- &TestProgress{
			Type:    "error",
			Message: "初始化模型失败: " + err.Error(),
		}
		return
	}

	if len(models) == 0 {
		run.ProgressChan <- &TestProgress{
			Type:    "error",
			Message: "模型初始化后无可用模型",
		}
		return
	}

	// 创建测试引擎（使用带进度回调的版本）
	testEngine := engine.NewTestEngineWithProgress(cfg.Test, models, cfg.Prompt, cfg.Proxies, func(p *engine.ProgressInfo) {
		// 检查是否取消
		select {
		case <-run.Ctx.Done():
			return
		default:
		}

		run.ProgressChan <- &TestProgress{
			Type:            "progress",
			ModelName:       p.ModelName,
			Concurrency:     p.Concurrency,
			ContextTokens:   p.ContextTokens,
			CurrentRequests: p.CurrentRequests,
			TotalRequests:   p.TotalRequests,
			SuccessRequests: p.SuccessRequests,
			FailedRequests:  p.FailedRequests,
			AvgTTFTMs:       p.AvgTTFTMs,
			PrefillTPS:      p.PrefillTPS,
			RPS:             p.RPS,
			TPS:             p.TPS,
			DecodeTPSAvg:    p.DecodeTPSAvg,
		}
	})

	// 运行测试
	results, err := testEngine.RunWithContext(run.Ctx)
	if err != nil {
		if run.Ctx.Err() != nil {
			run.ProgressChan <- &TestProgress{
				Type:    "stopped",
				Message: "测试已停止",
			}
		} else {
			run.ProgressChan <- &TestProgress{
				Type:    "error",
				Message: "测试执行失败: " + err.Error(),
			}
		}
		return
	}

	endTime := time.Now()

	// 保存结果到 Run
	run.ResultsMu.Lock()
	run.LatestResults = &TestResultStore{
		Results:   results,
		StartTime: startTime,
		EndTime:   endTime,
		Duration:  endTime.Sub(startTime).String(),
	}
	run.ResultsMu.Unlock()

	// 保存结果到文件
	store, _ := loadMachinesStore()
	machineID := run.MachineID
	backendID := run.BackendID

	// 获取机器名称
	machineName := ""
	if store != nil {
		for _, m := range store.Machines {
			if m.ID == machineID {
				machineName = m.Name
				break
			}
		}
	}

	// 获取后端名称
	backendName := ""
	backendStore, err := loadBackendsStore(machineID)
	if err == nil && backendStore != nil {
		for _, b := range backendStore.Backends {
			if b.ID == backendID {
				backendName = b.Name
				break
			}
		}
	}

	// 保存JSON结果
	if jsonPath, err := SaveTestResultsJSON(results, machineID, backendID, machineName, backendName); err == nil {
		log.Printf("测试结果已保存到: %s", jsonPath)
	} else {
		log.Printf("保存JSON结果失败: %v", err)
	}

	// 保存XLSX结果
	if xlsxPath, err := SaveTestResultsToXLSX(results, machineID, backendID, machineName); err == nil {
		log.Printf("XLSX报告已保存到: %s", xlsxPath)
	} else {
		log.Printf("保存XLSX报告失败: %v", err)
	}

	// 发送完成消息
	run.ProgressChan <- &TestProgress{
		Type:    "complete",
		Message: "测试完成",
	}
}
