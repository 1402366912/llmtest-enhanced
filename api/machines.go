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

// Machine 机器信息结构
type Machine struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	GPUModel    string    `json:"gpu_model,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// MachinesStore 机器列表存储
type MachinesStore struct {
	Machines        []Machine `json:"machines"`
	CurrentMachine  string    `json:"current_machine"`
}

const (
	dataDir        = "data"
	machinesFile   = "machines.json"
)

// getDataDir 获取数据目录路径
func getDataDir() string {
	return dataDir
}

// getMachinesFilePath 获取机器列表文件路径
func getMachinesFilePath() string {
	return filepath.Join(getDataDir(), machinesFile)
}

// getMachineDir 获取指定机器的数据目录
func getMachineDir(machineID string) string {
	return filepath.Join(getDataDir(), machineID)
}

// getMachineConfigPath 获取指定机器的配置文件路径
func getMachineConfigPath(machineID string) string {
	return filepath.Join(getMachineDir(machineID), "config.yaml")
}

// getMachineResultsDir 获取指定机器的结果目录
func getMachineResultsDir(machineID string) string {
	return filepath.Join(getMachineDir(machineID), "results")
}

// loadMachinesStore 加载机器列表
func loadMachinesStore() (*MachinesStore, error) {
	filePath := getMachinesFilePath()
	
	// 确保数据目录存在
	if err := os.MkdirAll(getDataDir(), 0755); err != nil {
		return nil, err
	}
	
	// 如果文件不存在，返回空存储
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return &MachinesStore{
			Machines:       []Machine{},
			CurrentMachine: "",
		}, nil
	}
	
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	
	var store MachinesStore
	if err := json.Unmarshal(data, &store); err != nil {
		return nil, err
	}
	
	return &store, nil
}

// saveMachinesStore 保存机器列表
func saveMachinesStore(store *MachinesStore) error {
	filePath := getMachinesFilePath()
	
	// 确保数据目录存在
	if err := os.MkdirAll(getDataDir(), 0755); err != nil {
		return err
	}
	
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	
	return os.WriteFile(filePath, data, 0644)
}

// initMachineDirectories 初始化机器目录结构
func initMachineDirectories(machineID string) error {
	machineDir := getMachineDir(machineID)
	resultsDir := getMachineResultsDir(machineID)
	
	// 创建机器目录
	if err := os.MkdirAll(machineDir, 0755); err != nil {
		return err
	}
	
	// 创建结果目录
	if err := os.MkdirAll(resultsDir, 0755); err != nil {
		return err
	}
	
	return nil
}

// getMachines 获取机器列表
func (s *Server) getMachines(c *gin.Context) {
	store, err := loadMachinesStore()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "加载机器列表失败: " + err.Error(),
		})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"machines":        store.Machines,
			"current_machine": store.CurrentMachine,
		},
	})
}

// createMachine 创建机器
func (s *Server) createMachine(c *gin.Context) {
	var req struct {
		Name        string `json:"name" binding:"required"`
		Description string `json:"description"`
		GPUModel    string `json:"gpu_model"`
	}
	
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "参数错误: " + err.Error(),
		})
		return
	}
	
	store, err := loadMachinesStore()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "加载机器列表失败: " + err.Error(),
		})
		return
	}
	
	// 生成唯一ID（使用时间戳）
	machineID := time.Now().Format("20060102150405")
	
	// 检查名称是否重复
	for _, m := range store.Machines {
		if m.Name == req.Name {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "机器名称已存在",
			})
			return
		}
	}
	
	now := time.Now()
	machine := Machine{
		ID:          machineID,
		Name:        req.Name,
		Description: req.Description,
		GPUModel:    req.GPUModel,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	
	// 初始化机器目录
	if err := initMachineDirectories(machineID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "创建机器目录失败: " + err.Error(),
		})
		return
	}
	
	// 复制默认配置到机器目录
	if err := s.copyDefaultConfig(machineID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "初始化配置失败: " + err.Error(),
		})
		return
	}
	
	store.Machines = append(store.Machines, machine)
	
	// 如果是第一台机器，自动设为当前机器
	if len(store.Machines) == 1 {
		store.CurrentMachine = machineID
	}
	
	if err := saveMachinesStore(store); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "保存机器列表失败: " + err.Error(),
		})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "机器创建成功",
		"data":    machine,
	})
}

// copyDefaultConfig 复制默认配置到机器目录
func (s *Server) copyDefaultConfig(machineID string) error {
	configPath := getMachineConfigPath(machineID)
	
	// 如果配置文件已存在，不覆盖
	if _, err := os.Stat(configPath); err == nil {
		return nil
	}
	
	// 读取当前配置
	s.mu.RLock()
	cfg := s.config
	s.mu.RUnlock()
	
	// 保存到机器目录
	return config.SaveConfig(cfg, configPath)
}

// updateMachine 更新机器
func (s *Server) updateMachine(c *gin.Context) {
	machineID := c.Param("id")
	
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		GPUModel    string `json:"gpu_model"`
	}
	
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "参数错误: " + err.Error(),
		})
		return
	}
	
	store, err := loadMachinesStore()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "加载机器列表失败: " + err.Error(),
		})
		return
	}
	
	found := false
	for i, m := range store.Machines {
		if m.ID == machineID {
			// 检查名称是否与其他机器重复
			if req.Name != "" && req.Name != m.Name {
				for _, other := range store.Machines {
					if other.ID != machineID && other.Name == req.Name {
						c.JSON(http.StatusBadRequest, gin.H{
							"success": false,
							"error":   "机器名称已存在",
						})
						return
					}
				}
				store.Machines[i].Name = req.Name
			}
			
			if req.Description != "" {
				store.Machines[i].Description = req.Description
			}
			if req.GPUModel != "" {
				store.Machines[i].GPUModel = req.GPUModel
			}
			store.Machines[i].UpdatedAt = time.Now()
			found = true
			break
		}
	}
	
	if !found {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "机器不存在",
		})
		return
	}
	
	if err := saveMachinesStore(store); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "保存机器列表失败: " + err.Error(),
		})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "机器更新成功",
	})
}

// deleteMachine 删除机器
func (s *Server) deleteMachine(c *gin.Context) {
	machineID := c.Param("id")
	
	store, err := loadMachinesStore()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "加载机器列表失败: " + err.Error(),
		})
		return
	}
	
	found := false
	for i, m := range store.Machines {
		if m.ID == machineID {
			store.Machines = append(store.Machines[:i], store.Machines[i+1:]...)
			found = true
			break
		}
	}
	
	if !found {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "机器不存在",
		})
		return
	}
	
	// 如果删除的是当前机器，切换到第一台机器
	if store.CurrentMachine == machineID {
		if len(store.Machines) > 0 {
			store.CurrentMachine = store.Machines[0].ID
		} else {
			store.CurrentMachine = ""
		}
	}
	
	// 删除机器目录（可选，这里保留数据）
	// os.RemoveAll(getMachineDir(machineID))
	
	if err := saveMachinesStore(store); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "保存机器列表失败: " + err.Error(),
		})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "机器删除成功",
	})
}

// selectMachine 切换当前机器
func (s *Server) selectMachine(c *gin.Context) {
	machineID := c.Param("id")
	
	store, err := loadMachinesStore()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "加载机器列表失败: " + err.Error(),
		})
		return
	}
	
	// 检查机器是否存在
	found := false
	for _, m := range store.Machines {
		if m.ID == machineID {
			found = true
			break
		}
	}
	
	if !found {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "机器不存在",
		})
		return
	}
	
	store.CurrentMachine = machineID
	
	if err := saveMachinesStore(store); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "保存机器列表失败: " + err.Error(),
		})
		return
	}
	
	// 加载该机器的配置
	configPath := getMachineConfigPath(machineID)
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
		"message": "已切换到机器",
		"data": gin.H{
			"machine_id": machineID,
		},
	})
}

// getMachineConfig 获取指定机器的配置
func (s *Server) getMachineConfig(c *gin.Context) {
	machineID := c.Param("id")
	
	configPath := getMachineConfigPath(machineID)
	
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "机器配置不存在",
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

// updateMachineConfig 更新指定机器的配置
func (s *Server) updateMachineConfig(c *gin.Context) {
	machineID := c.Param("id")
	
	var newConfig config.Config
	if err := c.ShouldBindJSON(&newConfig); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "参数错误: " + err.Error(),
		})
		return
	}
	
	// 确保机器目录存在
	if err := initMachineDirectories(machineID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "创建机器目录失败: " + err.Error(),
		})
		return
	}
	
	configPath := getMachineConfigPath(machineID)
	
	if err := config.SaveConfig(&newConfig, configPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "保存配置失败: " + err.Error(),
		})
		return
	}
	
	// 如果是当前机器，更新内存中的配置
	store, _ := loadMachinesStore()
	if store != nil && store.CurrentMachine == machineID {
		s.mu.Lock()
		s.config = &newConfig
		s.mu.Unlock()
	}
	
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "配置已更新",
	})
}

// getCurrentMachine 获取当前机器信息
func (s *Server) getCurrentMachine(c *gin.Context) {
	store, err := loadMachinesStore()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "加载机器列表失败: " + err.Error(),
		})
		return
	}
	
	if store.CurrentMachine == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    nil,
		})
		return
	}
	
	var currentMachine *Machine
	for _, m := range store.Machines {
		if m.ID == store.CurrentMachine {
			currentMachine = &m
			break
		}
	}
	
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    currentMachine,
	})
}

