package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lemonlinger/llm-test/config"
)

// Backend 后端信息结构
type Backend struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// BackendsStore 后端列表存储（每个机器一个）
type BackendsStore struct {
	Backends       []Backend `json:"backends"`
	CurrentBackend string    `json:"current_backend"`
}

const backendsFile = "backends.json"

// getBackendsFilePath 获取后端列表文件路径
func getBackendsFilePath(machineID string) string {
	return filepath.Join(getMachineDir(machineID), backendsFile)
}

// getBackendDir 获取指定后端的数据目录
func getBackendDir(machineID, backendID string) string {
	return filepath.Join(getMachineDir(machineID), backendID)
}

// getBackendConfigPath 获取指定后端的配置文件路径
func getBackendConfigPath(machineID, backendID string) string {
	return filepath.Join(getBackendDir(machineID, backendID), "config.yaml")
}

// getBackendResultsDir 获取指定后端的结果目录
func getBackendResultsDir(machineID, backendID string) string {
	return filepath.Join(getBackendDir(machineID, backendID), "results")
}

// loadBackendsStore 加载后端列表
func loadBackendsStore(machineID string) (*BackendsStore, error) {
	filePath := getBackendsFilePath(machineID)

	// 确保机器目录存在
	if err := os.MkdirAll(getMachineDir(machineID), 0755); err != nil {
		return nil, err
	}

	// 如果文件不存在，返回空存储
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return &BackendsStore{
			Backends:       []Backend{},
			CurrentBackend: "",
		}, nil
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var store BackendsStore
	if err := json.Unmarshal(data, &store); err != nil {
		return nil, err
	}

	return &store, nil
}

// saveBackendsStore 保存后端列表
func saveBackendsStore(machineID string, store *BackendsStore) error {
	filePath := getBackendsFilePath(machineID)

	// 确保机器目录存在
	if err := os.MkdirAll(getMachineDir(machineID), 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filePath, data, 0644)
}

// initBackendDirectories 初始化后端目录结构
func initBackendDirectories(machineID, backendID string) error {
	backendDir := getBackendDir(machineID, backendID)
	resultsDir := getBackendResultsDir(machineID, backendID)

	// 创建后端目录
	if err := os.MkdirAll(backendDir, 0755); err != nil {
		return err
	}

	// 创建结果目录
	if err := os.MkdirAll(resultsDir, 0755); err != nil {
		return err
	}

	return nil
}

// getBackends 获取后端列表
func (s *Server) getBackends(c *gin.Context) {
	machineID := c.Param("id")

	store, err := loadBackendsStore(machineID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "加载后端列表失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"backends":        store.Backends,
			"current_backend": store.CurrentBackend,
		},
	})
}

// createBackend 创建后端
func (s *Server) createBackend(c *gin.Context) {
	machineID := c.Param("id")

	var req struct {
		Name        string `json:"name" binding:"required"`
		Description string `json:"description"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "参数错误: " + err.Error(),
		})
		return
	}

	// 检查机器是否存在
	machinesStore, err := loadMachinesStore()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "加载机器列表失败: " + err.Error(),
		})
		return
	}

	machineExists := false
	for _, m := range machinesStore.Machines {
		if m.ID == machineID {
			machineExists = true
			break
		}
	}

	if !machineExists {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "机器不存在",
		})
		return
	}

	store, err := loadBackendsStore(machineID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "加载后端列表失败: " + err.Error(),
		})
		return
	}

	// 生成唯一ID（使用时间戳）
	backendID := time.Now().Format("20060102150405")

	// 检查名称是否重复
	for _, b := range store.Backends {
		if b.Name == req.Name {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "后端名称已存在",
			})
			return
		}
	}

	now := time.Now()
	backend := Backend{
		ID:          backendID,
		Name:        req.Name,
		Description: req.Description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	// 初始化后端目录
	if err := initBackendDirectories(machineID, backendID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "创建后端目录失败: " + err.Error(),
		})
		return
	}

	// 复制默认配置到后端目录
	if err := s.copyDefaultConfigToBackend(machineID, backendID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "初始化配置失败: " + err.Error(),
		})
		return
	}

	store.Backends = append(store.Backends, backend)

	// 如果是第一个后端，自动设为当前后端
	if len(store.Backends) == 1 {
		store.CurrentBackend = backendID
	}

	if err := saveBackendsStore(machineID, store); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "保存后端列表失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "后端创建成功",
		"data":    backend,
	})
}

// copyDefaultConfigToBackend 复制默认配置到后端目录
func (s *Server) copyDefaultConfigToBackend(machineID, backendID string) error {
	configPath := getBackendConfigPath(machineID, backendID)

	// 如果配置文件已存在，不覆盖
	if _, err := os.Stat(configPath); err == nil {
		return nil
	}

	// 首先尝试从机器目录复制配置
	machineConfigPath := getMachineConfigPath(machineID)
	if _, err := os.Stat(machineConfigPath); err == nil {
		cfg, err := config.LoadConfig(machineConfigPath)
		if err == nil {
			return config.SaveConfig(cfg, configPath)
		}
	}

	// 如果机器配置不存在，使用当前配置
	s.mu.RLock()
	cfg := s.config
	s.mu.RUnlock()

	return config.SaveConfig(cfg, configPath)
}

// updateBackend 更新后端
func (s *Server) updateBackend(c *gin.Context) {
	machineID := c.Param("id")
	backendID := c.Param("backend_id")

	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "参数错误: " + err.Error(),
		})
		return
	}

	store, err := loadBackendsStore(machineID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "加载后端列表失败: " + err.Error(),
		})
		return
	}

	found := false
	for i, b := range store.Backends {
		if b.ID == backendID {
			// 检查名称是否与其他后端重复
			if req.Name != "" && req.Name != b.Name {
				for _, other := range store.Backends {
					if other.ID != backendID && other.Name == req.Name {
						c.JSON(http.StatusBadRequest, gin.H{
							"success": false,
							"error":   "后端名称已存在",
						})
						return
					}
				}
				store.Backends[i].Name = req.Name
			}

			if req.Description != "" {
				store.Backends[i].Description = req.Description
			}
			store.Backends[i].UpdatedAt = time.Now()
			found = true
			break
		}
	}

	if !found {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "后端不存在",
		})
		return
	}

	if err := saveBackendsStore(machineID, store); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "保存后端列表失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "后端更新成功",
	})
}

// deleteBackend 删除后端
func (s *Server) deleteBackend(c *gin.Context) {
	machineID := c.Param("id")
	backendID := c.Param("backend_id")

	store, err := loadBackendsStore(machineID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "加载后端列表失败: " + err.Error(),
		})
		return
	}

	found := false
	for i, b := range store.Backends {
		if b.ID == backendID {
			store.Backends = append(store.Backends[:i], store.Backends[i+1:]...)
			found = true
			break
		}
	}

	if !found {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "后端不存在",
		})
		return
	}

	// 如果删除的是当前后端，切换到第一个后端
	if store.CurrentBackend == backendID {
		if len(store.Backends) > 0 {
			store.CurrentBackend = store.Backends[0].ID
		} else {
			store.CurrentBackend = ""
		}
	}

	// 删除后端目录（可选，这里保留数据）
	// os.RemoveAll(getBackendDir(machineID, backendID))

	if err := saveBackendsStore(machineID, store); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "保存后端列表失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "后端删除成功",
	})
}

// selectBackend 切换当前后端
func (s *Server) selectBackend(c *gin.Context) {
	machineID := c.Param("id")
	backendID := c.Param("backend_id")

	store, err := loadBackendsStore(machineID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "加载后端列表失败: " + err.Error(),
		})
		return
	}

	// 检查后端是否存在
	found := false
	for _, b := range store.Backends {
		if b.ID == backendID {
			found = true
			break
		}
	}

	if !found {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "后端不存在",
		})
		return
	}

	store.CurrentBackend = backendID

	if err := saveBackendsStore(machineID, store); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "保存后端列表失败: " + err.Error(),
		})
		return
	}

	// 加载该后端的配置
	configPath := getBackendConfigPath(machineID, backendID)
	if _, err := os.Stat(configPath); err == nil {
		cfg, err := config.LoadConfig(configPath)
		if err == nil {
			s.mu.Lock()
			s.config = cfg
			s.configPath = configPath
			s.mu.Unlock()
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "已切换到后端",
		"data": gin.H{
			"machine_id": machineID,
			"backend_id": backendID,
		},
	})
}

// getBackendConfig 获取指定后端的配置
func (s *Server) getBackendConfig(c *gin.Context) {
	machineID := c.Param("id")
	backendID := c.Param("backend_id")

	configPath := getBackendConfigPath(machineID, backendID)

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "后端配置不存在",
		})
		return
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "加载配置失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    cfg,
	})
}

// updateBackendConfig 更新指定后端的配置
func (s *Server) updateBackendConfig(c *gin.Context) {
	machineID := c.Param("id")
	backendID := c.Param("backend_id")

	var newConfig config.Config
	if err := c.ShouldBindJSON(&newConfig); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "参数错误: " + err.Error(),
		})
		return
	}

	// 根据起始和结束值重新生成上下文长度梯度
	// 只要设置了 start 和 end，就重新生成（覆盖旧值）
	if newConfig.Test.ContextTokenStart > 0 && newConfig.Test.ContextTokenEnd > 0 {
		newConfig.Test.ContextTokenLevels = config.GenerateContextLevels(newConfig.Test.ContextTokenStart, newConfig.Test.ContextTokenEnd)
	}

	// 确保后端目录存在
	if err := initBackendDirectories(machineID, backendID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "创建后端目录失败: " + err.Error(),
		})
		return
	}

	configPath := getBackendConfigPath(machineID, backendID)

	if err := config.SaveConfig(&newConfig, configPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "保存配置失败: " + err.Error(),
		})
		return
	}

	// 如果是当前后端，更新内存中的配置
	machinesStore, _ := loadMachinesStore()
	backendsStore, _ := loadBackendsStore(machineID)
	if machinesStore != nil && backendsStore != nil &&
		machinesStore.CurrentMachine == machineID &&
		backendsStore.CurrentBackend == backendID {
		s.mu.Lock()
		s.config = &newConfig
		s.mu.Unlock()
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "配置已更新",
	})
}

// getCurrentBackend 获取当前后端信息
func (s *Server) getCurrentBackend(c *gin.Context) {
	machineID := c.Param("id")

	store, err := loadBackendsStore(machineID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "加载后端列表失败: " + err.Error(),
		})
		return
	}

	if store.CurrentBackend == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    nil,
		})
		return
	}

	var currentBackend *Backend
	for _, b := range store.Backends {
		if b.ID == store.CurrentBackend {
			currentBackend = &b
			break
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    currentBackend,
	})
}

