package api

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// exportData 导出数据为ZIP文件
func (s *Server) exportData(c *gin.Context) {
	dataDir := "data"
	
	// 检查data目录是否存在
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "数据目录不存在",
		})
		return
	}

	// 获取自定义文件名，默认使用时间戳
	customName := c.Query("name")
	var filename string
	if customName != "" {
		// 清理文件名，移除不安全字符
		customName = strings.ReplaceAll(customName, "/", "_")
		customName = strings.ReplaceAll(customName, "\\", "_")
		customName = strings.ReplaceAll(customName, ":", "_")
		if !strings.HasSuffix(strings.ToLower(customName), ".zip") {
			customName += ".zip"
		}
		filename = customName
	} else {
		filename = fmt.Sprintf("llm-test-data-%s.zip", time.Now().Format("20060102_150405"))
	}
	
	// 设置响应头
	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	
	// 创建ZIP写入器
	zipWriter := zip.NewWriter(c.Writer)
	defer zipWriter.Close()
	
	// 递归遍历data目录
	err := filepath.Walk(dataDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		// 跳过目录本身，但需要处理目录
		if info.IsDir() {
			return nil
		}
		
		// 获取相对路径
		relPath, err := filepath.Rel(dataDir, path)
		if err != nil {
			return err
		}
		
		// 创建ZIP文件条目
		writer, err := zipWriter.Create(relPath)
		if err != nil {
			return err
		}
		
		// 读取并写入文件内容
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		
		_, err = io.Copy(writer, file)
		return err
	})
	
	if err != nil {
		log.Printf("导出数据失败: %v", err)
		// 注意：由于已经开始写入响应，这里无法返回JSON错误
		return
	}
	
	log.Printf("数据导出成功: %s", filename)
}

// ImportResult 导入结果
type ImportResult struct {
	MachinesAdded   int      `json:"machines_added"`
	MachinesMerged  int      `json:"machines_merged"`
	BackendsAdded   int      `json:"backends_added"`
	BackendsMerged  int      `json:"backends_merged"`
	ResultsAdded    int      `json:"results_added"`
	ResultsSkipped  int      `json:"results_skipped"`
	Errors          []string `json:"errors,omitempty"`
}

// importData 导入ZIP数据
func (s *Server) importData(c *gin.Context) {
	// 获取上传的文件
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "请上传ZIP文件",
		})
		return
	}
	defer file.Close()
	
	// 验证文件类型
	if !strings.HasSuffix(strings.ToLower(header.Filename), ".zip") {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "只支持ZIP文件",
		})
		return
	}
	
	// 创建临时目录
	tempDir, err := os.MkdirTemp("", "llm-test-import-*")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "创建临时目录失败: " + err.Error(),
		})
		return
	}
	defer os.RemoveAll(tempDir)
	
	// 保存上传的文件到临时目录
	tempZipPath := filepath.Join(tempDir, "import.zip")
	tempFile, err := os.Create(tempZipPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "保存临时文件失败: " + err.Error(),
		})
		return
	}
	
	_, err = io.Copy(tempFile, file)
	tempFile.Close()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "保存临时文件失败: " + err.Error(),
		})
		return
	}
	
	// 解压ZIP文件
	extractDir := filepath.Join(tempDir, "extracted")
	if err := unzipFile(tempZipPath, extractDir); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "解压文件失败: " + err.Error(),
		})
		return
	}
	
	// 执行合并
	result, err := s.mergeImportedData(extractDir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "合并数据失败: " + err.Error(),
		})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "数据导入成功",
		"data":    result,
	})
}

// unzipFile 解压ZIP文件到目标目录
func unzipFile(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()
	
	for _, f := range r.File {
		fpath := filepath.Join(dest, f.Name)
		
		// 安全检查：防止ZIP路径遍历攻击
		if !strings.HasPrefix(fpath, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("非法文件路径: %s", f.Name)
		}
		
		if f.FileInfo().IsDir() {
			os.MkdirAll(fpath, os.ModePerm)
			continue
		}
		
		// 创建目录
		if err := os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return err
		}
		
		// 创建文件
		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}
		
		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return err
		}
		
		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()
		
		if err != nil {
			return err
		}
	}
	
	return nil
}

// mergeImportedData 合并导入的数据
func (s *Server) mergeImportedData(importDir string) (*ImportResult, error) {
	result := &ImportResult{}
	dataDir := "data"
	
	// 确保本地data目录存在
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, err
	}
	
	// 读取导入的machines.json
	importMachinesPath := filepath.Join(importDir, "machines.json")
	importMachines, err := loadMachinesFile(importMachinesPath)
	if err != nil {
		return nil, fmt.Errorf("读取导入的machines.json失败: %v", err)
	}
	
	// 读取本地machines.json
	localMachinesPath := filepath.Join(dataDir, "machines.json")
	localMachines, err := loadMachinesFile(localMachinesPath)
	if err != nil {
		// 如果本地没有，创建空的
		localMachines = &MachinesData{Machines: []Machine{}}
	}
	
	// 建立名称到本地机器的映射
	localMachineByName := make(map[string]*Machine)
	for i := range localMachines.Machines {
		localMachineByName[localMachines.Machines[i].Name] = &localMachines.Machines[i]
	}
	
	// 处理每台导入的机器
	for _, importMachine := range importMachines.Machines {
		localMachine, exists := localMachineByName[importMachine.Name]
		
		var targetMachineID string
		if exists {
			// 机器已存在，使用本地ID
			targetMachineID = localMachine.ID
			result.MachinesMerged++
			log.Printf("合并机器: %s (使用本地ID: %s)", importMachine.Name, targetMachineID)
		} else {
			// 新机器，添加到本地
			targetMachineID = importMachine.ID
			localMachines.Machines = append(localMachines.Machines, importMachine)
			localMachineByName[importMachine.Name] = &importMachine
			result.MachinesAdded++
			log.Printf("添加新机器: %s (ID: %s)", importMachine.Name, targetMachineID)
		}
		
		// 处理该机器的后端
		importMachineDir := filepath.Join(importDir, importMachine.ID)
		localMachineDir := filepath.Join(dataDir, targetMachineID)
		
		// 确保本地机器目录存在
		os.MkdirAll(localMachineDir, 0755)
		
		// 合并后端
		backendsResult, err := s.mergeBackends(importMachineDir, localMachineDir)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("合并机器%s的后端失败: %v", importMachine.Name, err))
			continue
		}
		
		result.BackendsAdded += backendsResult.added
		result.BackendsMerged += backendsResult.merged
		result.ResultsAdded += backendsResult.resultsAdded
		result.ResultsSkipped += backendsResult.resultsSkipped
		
		// 复制机器配置（如果本地没有）
		importConfigPath := filepath.Join(importMachineDir, "config.yaml")
		localConfigPath := filepath.Join(localMachineDir, "config.yaml")
		if _, err := os.Stat(localConfigPath); os.IsNotExist(err) {
			if _, err := os.Stat(importConfigPath); err == nil {
				copyFile(importConfigPath, localConfigPath)
			}
		}
		
		// 合并机器根目录的results
		importResultsDir := filepath.Join(importMachineDir, "results")
		localResultsDir := filepath.Join(localMachineDir, "results")
		added, skipped := mergeResultsDir(importResultsDir, localResultsDir)
		result.ResultsAdded += added
		result.ResultsSkipped += skipped
	}
	
	// 保存更新后的machines.json
	if err := saveMachinesFile(localMachinesPath, localMachines); err != nil {
		return nil, fmt.Errorf("保存machines.json失败: %v", err)
	}
	
	return result, nil
}

type backendsMergeResult struct {
	added          int
	merged         int
	resultsAdded   int
	resultsSkipped int
}

// mergeBackends 合并后端
func (s *Server) mergeBackends(importMachineDir, localMachineDir string) (*backendsMergeResult, error) {
	result := &backendsMergeResult{}
	
	// 读取导入的backends.json
	importBackendsPath := filepath.Join(importMachineDir, "backends.json")
	importBackends, err := loadBackendsFile(importBackendsPath)
	if err != nil {
		// 没有backends.json，跳过
		return result, nil
	}
	
	// 读取本地backends.json
	localBackendsPath := filepath.Join(localMachineDir, "backends.json")
	localBackends, err := loadBackendsFile(localBackendsPath)
	if err != nil {
		localBackends = &BackendsData{Backends: []Backend{}}
	}
	
	// 建立名称到本地后端的映射
	localBackendByName := make(map[string]*Backend)
	for i := range localBackends.Backends {
		localBackendByName[localBackends.Backends[i].Name] = &localBackends.Backends[i]
	}
	
	// 处理每个导入的后端
	for _, importBackend := range importBackends.Backends {
		localBackend, exists := localBackendByName[importBackend.Name]
		
		var targetBackendID string
		if exists {
			// 后端已存在，使用本地ID
			targetBackendID = localBackend.ID
			result.merged++
			log.Printf("  合并后端: %s (使用本地ID: %s)", importBackend.Name, targetBackendID)
		} else {
			// 新后端，添加到本地
			targetBackendID = importBackend.ID
			localBackends.Backends = append(localBackends.Backends, importBackend)
			localBackendByName[importBackend.Name] = &importBackend
			result.added++
			log.Printf("  添加新后端: %s (ID: %s)", importBackend.Name, targetBackendID)
		}
		
		// 处理后端目录
		importBackendDir := filepath.Join(importMachineDir, importBackend.ID)
		localBackendDir := filepath.Join(localMachineDir, targetBackendID)
		
		// 确保本地后端目录存在
		os.MkdirAll(localBackendDir, 0755)
		
		// 复制后端配置（如果本地没有）
		importConfigPath := filepath.Join(importBackendDir, "config.yaml")
		localConfigPath := filepath.Join(localBackendDir, "config.yaml")
		if _, err := os.Stat(localConfigPath); os.IsNotExist(err) {
			if _, err := os.Stat(importConfigPath); err == nil {
				copyFile(importConfigPath, localConfigPath)
			}
		}
		
		// 合并测试结果
		importResultsDir := filepath.Join(importBackendDir, "results")
		localResultsDir := filepath.Join(localBackendDir, "results")
		added, skipped := mergeResultsDir(importResultsDir, localResultsDir)
		result.resultsAdded += added
		result.resultsSkipped += skipped
	}
	
	// 保存更新后的backends.json
	if err := saveBackendsFile(localBackendsPath, localBackends); err != nil {
		return result, err
	}
	
	return result, nil
}

// mergeResultsDir 合并测试结果目录
func mergeResultsDir(importDir, localDir string) (added, skipped int) {
	// 检查导入目录是否存在
	if _, err := os.Stat(importDir); os.IsNotExist(err) {
		return 0, 0
	}
	
	// 确保本地目录存在
	os.MkdirAll(localDir, 0755)
	
	// 遍历导入目录中的文件
	entries, err := os.ReadDir(importDir)
	if err != nil {
		return 0, 0
	}
	
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		
		filename := entry.Name()
		importPath := filepath.Join(importDir, filename)
		localPath := filepath.Join(localDir, filename)
		
		// 检查本地是否已存在
		if _, err := os.Stat(localPath); err == nil {
			// 已存在，跳过
			skipped++
			log.Printf("    跳过已存在的结果: %s", filename)
		} else {
			// 不存在，复制
			if err := copyFile(importPath, localPath); err == nil {
				added++
				log.Printf("    添加新结果: %s", filename)
			}
		}
	}
	
	return added, skipped
}

// copyFile 复制文件
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()
	
	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()
	
	_, err = io.Copy(destFile, sourceFile)
	return err
}

// MachinesData machines.json结构
type MachinesData struct {
	Machines       []Machine `json:"machines"`
	CurrentMachine string    `json:"current_machine"`
}

// BackendsData backends.json结构
type BackendsData struct {
	Backends       []Backend `json:"backends"`
	CurrentBackend string    `json:"current_backend"`
}

// loadMachinesFile 加载machines.json
func loadMachinesFile(path string) (*MachinesData, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	
	var machines MachinesData
	if err := json.Unmarshal(data, &machines); err != nil {
		return nil, err
	}
	
	return &machines, nil
}

// saveMachinesFile 保存machines.json
func saveMachinesFile(path string, data *MachinesData) error {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	
	return os.WriteFile(path, jsonData, 0644)
}

// loadBackendsFile 加载backends.json
func loadBackendsFile(path string) (*BackendsData, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	
	var backends BackendsData
	if err := json.Unmarshal(data, &backends); err != nil {
		return nil, err
	}
	
	return &backends, nil
}

// saveBackendsFile 保存backends.json
func saveBackendsFile(path string, data *BackendsData) error {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	
	return os.WriteFile(path, jsonData, 0644)
}

