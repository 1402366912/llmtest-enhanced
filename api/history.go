package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lemonlinger/llm-test/engine"
)

// HistoryRecord 历史记录结构
type HistoryRecord struct {
	ID          string                        `json:"id"`
	MachineID   string                        `json:"machine_id"`
	MachineName string                        `json:"machine_name"`
	BackendID   string                        `json:"backend_id,omitempty"`
	BackendName string                        `json:"backend_name,omitempty"`
	FileName    string                        `json:"file_name"`
	FilePath    string                        `json:"file_path"`
	CreatedAt   time.Time                     `json:"created_at"`
	Results     map[string]*engine.TestResult `json:"results,omitempty"`
	Summary     *HistorySummary               `json:"summary,omitempty"`
}

// HistorySummary 历史记录摘要
type HistorySummary struct {
	TotalRequests   int     `json:"total_requests"`
	SuccessRequests int     `json:"success_requests"`
	ModelCount      int     `json:"model_count"`
	MaxTPS          float64 `json:"max_tps"`
	MaxRPS          float64 `json:"max_rps"`
}

// LeaderboardEntry 天梯图条目
type LeaderboardEntry struct {
	Rank           int     `json:"rank"`
	MachineID      string  `json:"machine_id"`
	MachineName    string  `json:"machine_name"`
	BackendID      string  `json:"backend_id"`
	BackendName    string  `json:"backend_name"`
	GPUModel       string  `json:"gpu_model"`
	ModelName      string  `json:"model_name"`
	Concurrency    int     `json:"concurrency"`
	ContextTokens  int     `json:"context_tokens"`
	TotalRequests  int     `json:"total_requests"`
	TPS            float64 `json:"tps"`
	RPS            float64 `json:"rps"`
	AvgLatency     float64 `json:"avg_latency_ms"`
	TTFT           float64 `json:"ttft_ms"`
	SuccessRate    float64 `json:"success_rate"`
}

// getHistory 获取历史记录列表
func (s *Server) getHistory(c *gin.Context) {
	machineID := c.Query("machine_id")
	backendID := c.Query("backend_id")

	var records []HistoryRecord

	// 加载机器列表以获取机器名称
	machinesStore, _ := loadMachinesStore()
	getMachineName := func(id string) string {
		if machinesStore == nil {
			return ""
		}
		for _, m := range machinesStore.Machines {
			if m.ID == id {
				return m.Name
			}
		}
		return ""
	}

	// 获取后端名称
	getBackendName := func(machineId, backendId string) string {
		backendStore, err := loadBackendsStore(machineId)
		if err != nil || backendStore == nil {
			return ""
		}
		for _, b := range backendStore.Backends {
			if b.ID == backendId {
				return b.Name
			}
		}
		return ""
	}

	if machineID != "" && backendID != "" {
		// 获取指定机器指定后端的历史记录
		backendRecords, err := getHistoryForBackend(machineID, backendID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "获取历史记录失败: " + err.Error(),
			})
			return
		}
		// 补充机器和后端名称
		machineName := getMachineName(machineID)
		backendName := getBackendName(machineID, backendID)
		for i := range backendRecords {
			backendRecords[i].MachineName = machineName
			backendRecords[i].BackendName = backendName
		}
		records = backendRecords
	} else if machineID != "" {
		// 获取指定机器所有后端的历史记录
		machineName := getMachineName(machineID)
		backendStore, err := loadBackendsStore(machineID)
		if err == nil && backendStore != nil && len(backendStore.Backends) > 0 {
			for _, backend := range backendStore.Backends {
				backendRecords, err := getHistoryForBackend(machineID, backend.ID)
				if err != nil {
					continue
				}
				// 补充机器和后端名称
				for i := range backendRecords {
					backendRecords[i].MachineName = machineName
					backendRecords[i].BackendName = backend.Name
				}
				records = append(records, backendRecords...)
			}
		}
		// 如果没有后端，也尝试读取机器目录（兼容旧数据）
		if len(records) == 0 {
			machineRecords, err := getHistoryForMachine(machineID)
			if err == nil {
				for i := range machineRecords {
					machineRecords[i].MachineName = machineName
				}
				records = machineRecords
			}
		}
	} else {
		// 获取所有机器的历史记录
		if machinesStore == nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "加载机器列表失败",
			})
			return
		}

		for _, machine := range machinesStore.Machines {
			// 获取该机器所有后端的历史记录
			backendStore, err := loadBackendsStore(machine.ID)
			if err == nil && backendStore != nil && len(backendStore.Backends) > 0 {
				for _, backend := range backendStore.Backends {
					backendRecords, err := getHistoryForBackend(machine.ID, backend.ID)
					if err != nil {
						continue
					}
					// 补充机器和后端名称
					for i := range backendRecords {
						backendRecords[i].MachineName = machine.Name
						backendRecords[i].BackendName = backend.Name
					}
					records = append(records, backendRecords...)
				}
			}
		}
	}

	// 按创建时间倒序排序
	sort.Slice(records, func(i, j int) bool {
		return records[i].CreatedAt.After(records[j].CreatedAt)
	})

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    records,
	})
}

// getHistoryForMachine 获取指定机器的历史记录
func getHistoryForMachine(machineID string) ([]HistoryRecord, error) {
	resultsDir := getMachineResultsDir(machineID)

	// 检查目录是否存在
	if _, err := os.Stat(resultsDir); os.IsNotExist(err) {
		return []HistoryRecord{}, nil
	}

	var records []HistoryRecord

	// 遍历结果目录
	entries, err := os.ReadDir(resultsDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".xlsx") && !strings.HasSuffix(name, ".json") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		// 从文件名解析时间
		// 格式: test_result_20060102_150405.xlsx
		var createdAt time.Time
		if strings.HasPrefix(name, "test_result_") {
			timeStr := strings.TrimPrefix(name, "test_result_")
			timeStr = strings.TrimSuffix(timeStr, ".xlsx")
			timeStr = strings.TrimSuffix(timeStr, ".json")
			if t, err := time.Parse("20060102_150405", timeStr); err == nil {
				createdAt = t
			} else {
				createdAt = info.ModTime()
			}
		} else {
			createdAt = info.ModTime()
		}

		record := HistoryRecord{
			ID:        strings.TrimSuffix(name, filepath.Ext(name)),
			MachineID: machineID,
			FileName:  name,
			FilePath:  filepath.Join(resultsDir, name),
			CreatedAt: createdAt,
		}

		// 尝试加载JSON结果摘要
		jsonPath := filepath.Join(resultsDir, strings.TrimSuffix(name, ".xlsx")+".json")
		if _, err := os.Stat(jsonPath); err == nil {
			if summary, err := loadHistorySummary(jsonPath); err == nil {
				record.Summary = summary
			}
		}

		records = append(records, record)
	}

	return records, nil
}

// getHistoryForBackend 获取指定后端的历史记录
func getHistoryForBackend(machineID, backendID string) ([]HistoryRecord, error) {
	resultsDir := getBackendResultsDir(machineID, backendID)

	// 检查目录是否存在
	if _, err := os.Stat(resultsDir); os.IsNotExist(err) {
		return []HistoryRecord{}, nil
	}

	var records []HistoryRecord

	// 遍历结果目录
	entries, err := os.ReadDir(resultsDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".xlsx") && !strings.HasSuffix(name, ".json") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		// 从文件名解析时间
		// 格式: test_result_20060102_150405.xlsx
		var createdAt time.Time
		if strings.HasPrefix(name, "test_result_") {
			timeStr := strings.TrimPrefix(name, "test_result_")
			timeStr = strings.TrimSuffix(timeStr, ".xlsx")
			timeStr = strings.TrimSuffix(timeStr, ".json")
			if t, err := time.Parse("20060102_150405", timeStr); err == nil {
				createdAt = t
			} else {
				createdAt = info.ModTime()
			}
		} else {
			createdAt = info.ModTime()
		}

		record := HistoryRecord{
			ID:        strings.TrimSuffix(name, filepath.Ext(name)),
			MachineID: machineID,
			BackendID: backendID,
			FileName:  name,
			FilePath:  filepath.Join(resultsDir, name),
			CreatedAt: createdAt,
		}

		// 尝试加载JSON结果摘要
		jsonPath := filepath.Join(resultsDir, strings.TrimSuffix(name, ".xlsx")+".json")
		if _, err := os.Stat(jsonPath); err == nil {
			if summary, err := loadHistorySummary(jsonPath); err == nil {
				record.Summary = summary
			}
		}

		records = append(records, record)
	}

	return records, nil
}

// loadHistorySummary 加载历史摘要
func loadHistorySummary(jsonPath string) (*HistorySummary, error) {
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, err
	}

	var results map[string]*engine.TestResult
	if err := json.Unmarshal(data, &results); err != nil {
		return nil, err
	}

	summary := &HistorySummary{}
	modelSet := make(map[string]bool)

	for _, result := range results {
		summary.TotalRequests += result.TotalRequests
		summary.SuccessRequests += result.SuccessRequests
		modelSet[result.ModelName] = true

		if result.TokensPerSec > summary.MaxTPS {
			summary.MaxTPS = result.TokensPerSec
		}
		if result.RequestsPerSec > summary.MaxRPS {
			summary.MaxRPS = result.RequestsPerSec
		}
	}

	summary.ModelCount = len(modelSet)
	return summary, nil
}

// getHistoryDetail 获取历史记录详情
func (s *Server) getHistoryDetail(c *gin.Context) {
	historyID := c.Param("id")
	machineID := c.Query("machine_id")
	backendID := c.Query("backend_id")

	if machineID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "缺少 machine_id 参数",
		})
		return
	}

	// 根据是否有 backend_id 决定结果目录
	var resultsDir string
	if backendID != "" {
		resultsDir = getBackendResultsDir(machineID, backendID)
	} else {
		resultsDir = getMachineResultsDir(machineID)
	}
	jsonPath := filepath.Join(resultsDir, historyID+".json")

	// 尝试读取JSON结果
	if _, err := os.Stat(jsonPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "历史记录不存在",
		})
		return
	}

	data, err := os.ReadFile(jsonPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "读取历史记录失败: " + err.Error(),
		})
		return
	}

	var results map[string]*engine.TestResult
	if err := json.Unmarshal(data, &results); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "解析历史记录失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    results,
	})
}

// deleteHistory 删除历史记录
func (s *Server) deleteHistory(c *gin.Context) {
	historyID := c.Param("id")
	machineID := c.Query("machine_id")
	backendID := c.Query("backend_id")

	if machineID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "缺少 machine_id 参数",
		})
		return
	}

	// 根据是否有 backend_id 决定结果目录
	var resultsDir string
	if backendID != "" {
		resultsDir = getBackendResultsDir(machineID, backendID)
	} else {
		resultsDir = getMachineResultsDir(machineID)
	}

	// 删除相关文件
	xlsxPath := filepath.Join(resultsDir, historyID+".xlsx")
	jsonPath := filepath.Join(resultsDir, historyID+".json")

	os.Remove(xlsxPath)
	os.Remove(jsonPath)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "历史记录已删除",
	})
}

// exportHistory 导出历史记录
func (s *Server) exportHistory(c *gin.Context) {
	historyID := c.Param("id")
	machineID := c.Query("machine_id")
	backendID := c.Query("backend_id")

	if machineID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "缺少 machine_id 参数",
		})
		return
	}

	// 根据是否有 backend_id 决定结果目录
	var resultsDir string
	if backendID != "" {
		resultsDir = getBackendResultsDir(machineID, backendID)
	} else {
		resultsDir = getMachineResultsDir(machineID)
	}
	xlsxPath := filepath.Join(resultsDir, historyID+".xlsx")

	// 检查文件是否存在
	if _, err := os.Stat(xlsxPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "XLSX文件不存在",
		})
		return
	}

	// 返回文件下载
	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Disposition", "attachment; filename="+historyID+".xlsx")
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.File(xlsxPath)
}

// getLeaderboard 获取天梯图数据
// 从所有机器的所有后端中获取测试数据，按机器+后端组合展示
// 支持灵活筛选：可选模型、并发数、上下文长度
func (s *Server) getLeaderboard(c *gin.Context) {
	metric := c.DefaultQuery("metric", "tps") // tps, rps, latency, ttft
	modelName := c.Query("model")             // 可选：筛选特定模型

	// 可选筛选参数
	var filterConcurrency, filterContextTokens int
	if concStr := c.Query("concurrency"); concStr != "" {
		if v, err := strconv.Atoi(concStr); err == nil {
			filterConcurrency = v
		}
	}
	if ctxStr := c.Query("context_tokens"); ctxStr != "" {
		if v, err := strconv.Atoi(ctxStr); err == nil {
			filterContextTokens = v
		}
	}

	store, err := loadMachinesStore()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "加载机器列表失败: " + err.Error(),
		})
		return
	}

	var entries []LeaderboardEntry

	// 收集所有可用的筛选选项
	allModels := make(map[string]bool)
	allConcurrencies := make(map[int]bool)
	allContextTokens := make(map[int]bool)

	// 遍历所有机器的所有后端获取测试结果
	for _, machine := range store.Machines {
		// 加载该机器的后端列表
		backendStore, err := loadBackendsStore(machine.ID)
		if err != nil {
			continue
		}

		for _, backend := range backendStore.Backends {
			// 获取该后端的所有测试结果（从所有JSON文件）
			results := getAllResultsForLeaderboard(machine.ID, backend.ID)
			if len(results) == 0 {
				continue
			}

			for _, result := range results {
				// 收集所有可用的筛选选项
				allModels[result.ModelName] = true
				allConcurrencies[result.ConcurrencyLevel] = true
				allContextTokens[result.ContextTargetTokens] = true

				// 应用筛选条件
				if modelName != "" && result.ModelName != modelName {
					continue
				}
				if filterConcurrency > 0 && result.ConcurrencyLevel != filterConcurrency {
					continue
				}
				if filterContextTokens > 0 {
					// 上下文长度允许 ±5% 容差
					tolerance := filterContextTokens / 20
					if result.ContextTargetTokens < filterContextTokens-tolerance ||
						result.ContextTargetTokens > filterContextTokens+tolerance {
						continue
					}
				}

				successRate := float64(0)
				if result.TotalRequests > 0 {
					successRate = float64(result.SuccessRequests) / float64(result.TotalRequests) * 100
				}

				entry := LeaderboardEntry{
					MachineID:     machine.ID,
					MachineName:   machine.Name,
					BackendID:     backend.ID,
					BackendName:   backend.Name,
					GPUModel:      machine.GPUModel,
					ModelName:     result.ModelName,
					Concurrency:   result.ConcurrencyLevel,
					ContextTokens: result.ContextTargetTokens,
					TotalRequests: result.TotalRequests,
					TPS:           result.TokensPerSec,
					RPS:           result.RequestsPerSec,
					AvgLatency:    float64(result.AvgLatency.Milliseconds()),
					TTFT:          float64(result.AvgFirstTokenLatency.Milliseconds()),
					SuccessRate:   successRate,
				}

				entries = append(entries, entry)
			}
		}
	}

	// 根据指标排序
	sort.Slice(entries, func(i, j int) bool {
		switch metric {
		case "tps":
			return entries[i].TPS > entries[j].TPS
		case "rps":
			return entries[i].RPS > entries[j].RPS
		case "latency":
			return entries[i].AvgLatency < entries[j].AvgLatency // 延迟越低越好
		case "ttft":
			return entries[i].TTFT < entries[j].TTFT // TTFT越低越好
		default:
			return entries[i].TPS > entries[j].TPS
		}
	})

	// 添加排名
	for i := range entries {
		entries[i].Rank = i + 1
	}

	// 转换模型列表
	modelList := make([]string, 0, len(allModels))
	for m := range allModels {
		modelList = append(modelList, m)
	}
	sort.Strings(modelList)

	// 转换并发数列表
	concurrencyList := make([]int, 0, len(allConcurrencies))
	for c := range allConcurrencies {
		concurrencyList = append(concurrencyList, c)
	}
	sort.Ints(concurrencyList)

	// 转换上下文长度列表
	contextList := make([]int, 0, len(allContextTokens))
	for c := range allContextTokens {
		contextList = append(contextList, c)
	}
	sort.Ints(contextList)

	c.JSON(http.StatusOK, gin.H{
		"success":               true,
		"data":                  entries,
		"metric":                metric,
		"available_models":      modelList,
		"available_concurrencies": concurrencyList,
		"available_context_tokens": contextList,
		"filter": gin.H{
			"concurrency":    filterConcurrency,
			"context_tokens": filterContextTokens,
			"model":          modelName,
		},
	})
}

// getAllResultsForLeaderboard 获取指定机器和后端的所有测试结果（用于天梯图）
// 同时读取后端目录和机器根目录（兼容旧数据）
func getAllResultsForLeaderboard(machineID, backendID string) []*engine.TestResult {
	var allResults []*engine.TestResult
	seen := make(map[string]bool) // 用于去重：机器+后端+模型+并发+上下文

	// 读取后端目录的结果
	backendResults := readAllJSONResultsFromDir(getBackendResultsDir(machineID, backendID))
	for _, result := range backendResults {
		key := fmt.Sprintf("%s-%s-%d-%d", machineID, result.ModelName, result.ConcurrencyLevel, result.ContextTargetTokens)
		if !seen[key] {
			seen[key] = true
			allResults = append(allResults, result)
		}
	}

	// 读取机器根目录的结果（兼容旧数据）
	machineResults := readAllJSONResultsFromDir(getMachineResultsDir(machineID))
	for _, result := range machineResults {
		key := fmt.Sprintf("%s-%s-%d-%d", machineID, result.ModelName, result.ConcurrencyLevel, result.ContextTargetTokens)
		if !seen[key] {
			seen[key] = true
			allResults = append(allResults, result)
		}
	}

	return allResults
}

// readAllJSONResultsFromDir 从目录读取所有JSON文件中的测试结果
func readAllJSONResultsFromDir(resultsDir string) []*engine.TestResult {
	var allResults []*engine.TestResult

	// 检查目录是否存在
	if _, err := os.Stat(resultsDir); os.IsNotExist(err) {
		return allResults
	}

	entries, err := os.ReadDir(resultsDir)
	if err != nil {
		return allResults
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		jsonPath := filepath.Join(resultsDir, entry.Name())
		data, err := os.ReadFile(jsonPath)
		if err != nil {
			continue
		}

		var results map[string]*engine.TestResult
		if err := json.Unmarshal(data, &results); err != nil {
			continue
		}

		for _, result := range results {
			if result != nil {
				allResults = append(allResults, result)
			}
		}
	}

	return allResults
}

// getLatestResultsForBackend 获取指定机器和后端的最新测试结果
func getLatestResultsForBackend(machineID, backendID string) (map[string]*engine.TestResult, error) {
	resultsDir := getBackendResultsDir(machineID, backendID)

	// 检查目录是否存在
	if _, err := os.Stat(resultsDir); os.IsNotExist(err) {
		return nil, nil
	}

	// 查找最新的JSON结果文件
	entries, err := os.ReadDir(resultsDir)
	if err != nil {
		return nil, err
	}

	var latestFile string
	var latestTime time.Time

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if info.ModTime().After(latestTime) {
			latestTime = info.ModTime()
			latestFile = entry.Name()
		}
	}

	if latestFile == "" {
		return nil, nil
	}

	// 读取最新结果
	jsonPath := filepath.Join(resultsDir, latestFile)
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, err
	}

	var results map[string]*engine.TestResult
	if err := json.Unmarshal(data, &results); err != nil {
		return nil, err
	}

	return results, nil
}

// getLatestResultsForMachine 获取机器最新的测试结果
func getLatestResultsForMachine(machineID string) (map[string]*engine.TestResult, error) {
	resultsDir := getMachineResultsDir(machineID)

	// 检查目录是否存在
	if _, err := os.Stat(resultsDir); os.IsNotExist(err) {
		return nil, nil
	}

	// 查找最新的JSON结果文件
	entries, err := os.ReadDir(resultsDir)
	if err != nil {
		return nil, err
	}

	var latestFile string
	var latestTime time.Time

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if info.ModTime().After(latestTime) {
			latestTime = info.ModTime()
			latestFile = entry.Name()
		}
	}

	if latestFile == "" {
		return nil, nil
	}

	// 读取最新结果
	jsonPath := filepath.Join(resultsDir, latestFile)
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, err
	}

	var results map[string]*engine.TestResult
	if err := json.Unmarshal(data, &results); err != nil {
		return nil, err
	}

	return results, nil
}

// SaveTestResultsJSON 保存测试结果为JSON文件（用于历史记录）
// 文件命名格式：{机器名}-{后端名}-{模型名}-{日期时间}.json
func SaveTestResultsJSON(results map[string]*engine.TestResult, machineID, backendID, machineName, backendName string) (string, error) {
	var resultsDir string
	if backendID != "" {
		resultsDir = getBackendResultsDir(machineID, backendID)
	} else {
		resultsDir = getMachineResultsDir(machineID)
	}

	// 确保目录存在
	if err := os.MkdirAll(resultsDir, 0755); err != nil {
		return "", err
	}

	// 生成文件名：{机器名}-{后端名}-{模型名}-{日期时间}.json
	fileName := generateResultFileName(results, machineName, backendName, ".json")
	filePath := filepath.Join(resultsDir, fileName)

	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return "", err
	}

	return filePath, nil
}

// generateResultFileName 生成结果文件名
// 格式：{机器名}-{后端名}-{模型名}-{日期时间}{后缀}
func generateResultFileName(results map[string]*engine.TestResult, machineName, backendName, suffix string) string {
	// 提取模型名（如果有多个模型，取第一个）
	modelName := "unknown"
	for _, result := range results {
		if result != nil && result.ModelName != "" {
			modelName = result.ModelName
			break
		}
	}

	// 清理名称中的非法字符
	cleanName := func(name string) string {
		// 替换文件名非法字符
		replacer := strings.NewReplacer(
			"/", "-",
			"\\", "-",
			":", "-",
			"*", "",
			"?", "",
			"\"", "",
			"<", "",
			">", "",
			"|", "",
			" ", "_",
		)
		return replacer.Replace(name)
	}

	// 生成时间戳
	timestamp := time.Now().Format("20060102_150405")

	// 组合文件名
	parts := make([]string, 0, 4)
	if machineName != "" {
		parts = append(parts, cleanName(machineName))
	}
	if backendName != "" {
		parts = append(parts, cleanName(backendName))
	}
	if modelName != "" && modelName != "unknown" {
		parts = append(parts, cleanName(modelName))
	}
	parts = append(parts, timestamp)

	return strings.Join(parts, "-") + suffix
}

// AnalyticsData 分析数据结构
type AnalyticsData struct {
	MachineID   string                `json:"machine_id"`
	MachineName string                `json:"machine_name"`
	BackendID   string                `json:"backend_id"`
	BackendName string                `json:"backend_name"`
	ModelName   string                `json:"model_name"`
	Data        []AnalyticsDataPoint  `json:"data"`
}

// AnalyticsDataPoint 单个数据点
type AnalyticsDataPoint struct {
	Concurrency   int     `json:"concurrency"`
	ContextTokens int     `json:"context_tokens"`
	TPS           float64 `json:"tps"`
	RPS           float64 `json:"rps"`
	AvgLatencyMs  float64 `json:"avg_latency_ms"`
	SuccessRate   float64 `json:"success_rate"`
}

// getAnalyticsData 获取指定机器/后端/模型的分析数据
func (s *Server) getAnalyticsData(c *gin.Context) {
	machineID := c.Query("machine_id")
	backendID := c.Query("backend_id")
	modelName := c.Query("model")

	if machineID == "" || backendID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "缺少 machine_id 或 backend_id 参数",
		})
		return
	}

	// 获取机器信息
	store, err := loadMachinesStore()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "加载机器列表失败: " + err.Error(),
		})
		return
	}

	var machineName string
	for _, m := range store.Machines {
		if m.ID == machineID {
			machineName = m.Name
			break
		}
	}

	// 获取后端信息
	backendStore, err := loadBackendsStore(machineID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "加载后端列表失败: " + err.Error(),
		})
		return
	}

	var backendName string
	for _, b := range backendStore.Backends {
		if b.ID == backendID {
			backendName = b.Name
			break
		}
	}

	// 获取最新测试结果
	results, err := getLatestResultsForBackend(machineID, backendID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取测试结果失败: " + err.Error(),
		})
		return
	}

	if results == nil {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    nil,
		})
		return
	}

	// 提取数据点
	var dataPoints []AnalyticsDataPoint
	for _, result := range results {
		// 如果指定了模型，则只返回该模型的数据
		if modelName != "" && result.ModelName != modelName {
			continue
		}

		successRate := float64(0)
		if result.TotalRequests > 0 {
			successRate = float64(result.SuccessRequests) / float64(result.TotalRequests) * 100
		}

		dataPoints = append(dataPoints, AnalyticsDataPoint{
			Concurrency:   result.ConcurrencyLevel,
			ContextTokens: result.ContextTargetTokens,
			TPS:           result.TokensPerSec,
			RPS:           result.RequestsPerSec,
			AvgLatencyMs:  float64(result.AvgLatency.Milliseconds()),
			SuccessRate:   successRate,
		})
	}

	// 按并发数和上下文长度排序
	sort.Slice(dataPoints, func(i, j int) bool {
		if dataPoints[i].Concurrency != dataPoints[j].Concurrency {
			return dataPoints[i].Concurrency < dataPoints[j].Concurrency
		}
		return dataPoints[i].ContextTokens < dataPoints[j].ContextTokens
	})

	// 确定返回的模型名
	returnModelName := modelName
	if returnModelName == "" && len(results) > 0 {
		for _, r := range results {
			returnModelName = r.ModelName
			break
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": AnalyticsData{
			MachineID:   machineID,
			MachineName: machineName,
			BackendID:   backendID,
			BackendName: backendName,
			ModelName:   returnModelName,
			Data:        dataPoints,
		},
	})
}

// getAvailableModels 获取所有机器/后端中可用的模型列表
func (s *Server) getAvailableModels(c *gin.Context) {
	store, err := loadMachinesStore()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "加载机器列表失败: " + err.Error(),
		})
		return
	}

	modelSet := make(map[string]bool)

	// 遍历所有机器和后端
	for _, machine := range store.Machines {
		backendStore, err := loadBackendsStore(machine.ID)
		if err != nil {
			continue
		}

		for _, backend := range backendStore.Backends {
			results, err := getLatestResultsForBackend(machine.ID, backend.ID)
			if err != nil || results == nil {
				continue
			}

			for _, result := range results {
				modelSet[result.ModelName] = true
			}
		}
	}

	// 转换为列表并排序
	models := make([]string, 0, len(modelSet))
	for m := range modelSet {
		models = append(models, m)
	}
	sort.Strings(models)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    models,
	})
}

