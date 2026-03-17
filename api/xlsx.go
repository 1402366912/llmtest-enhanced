package api

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/lemonlinger/llm-test/engine"
	"github.com/xuri/excelize/v2"
)

// XLSXExporter XLSX导出器
type XLSXExporter struct {
	file *excelize.File
}

// NewXLSXExporter 创建新的XLSX导出器
func NewXLSXExporter() *XLSXExporter {
	f := excelize.NewFile()
	return &XLSXExporter{file: f}
}

// ExportTestResults 导出测试结果到XLSX
func (e *XLSXExporter) ExportTestResults(results map[string]*engine.TestResult, machineID, machineName string) error {
	// 创建概览表
	if err := e.createSummarySheet(results, machineName); err != nil {
		return err
	}

	// 创建详细数据表
	if err := e.createDetailSheet(results); err != nil {
		return err
	}

	// 创建延迟分布表
	if err := e.createLatencySheet(results); err != nil {
		return err
	}

	return nil
}

// createSummarySheet 创建概览表
func (e *XLSXExporter) createSummarySheet(results map[string]*engine.TestResult, machineName string) error {
	sheetName := "测试概览"
	index, err := e.file.NewSheet(sheetName)
	if err != nil {
		return err
	}
	e.file.SetActiveSheet(index)

	// 删除默认的Sheet1
	e.file.DeleteSheet("Sheet1")

	// 设置标题行
	headers := []string{
		"模型名称", "并发度", "上下文Token", "总请求数", "成功请求数", "失败请求数",
		"成功率(%)", "平均延迟(ms)", "首Token延迟(ms)", "RPS", "TPS",
		"Prefill TPS", "Decode TPS", "平均输入Token", "平均输出Token",
	}

	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		e.file.SetCellValue(sheetName, cell, h)
	}

	// 设置标题行样式
	headerStyle, _ := e.file.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"4472C4"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center"},
	})
	e.file.SetRowStyle(sheetName, 1, 1, headerStyle)

	// 填充数据
	row := 2
	for _, result := range results {
		successRate := float64(0)
		if result.TotalRequests > 0 {
			successRate = float64(result.SuccessRequests) / float64(result.TotalRequests) * 100
		}

		data := []interface{}{
			result.ModelName,
			result.ConcurrencyLevel,
			result.ContextTargetTokens,
			result.TotalRequests,
			result.SuccessRequests,
			result.FailedRequests,
			fmt.Sprintf("%.2f", successRate),
			fmt.Sprintf("%.2f", float64(result.AvgLatency.Milliseconds())),
			fmt.Sprintf("%.2f", float64(result.AvgFirstTokenLatency.Milliseconds())),
			fmt.Sprintf("%.2f", result.RequestsPerSec),
			fmt.Sprintf("%.2f", result.TokensPerSec),
			fmt.Sprintf("%.2f", result.PrefillTokensPerSecAvg),
			fmt.Sprintf("%.2f", result.DecodeTokensPerSecAvg),
			fmt.Sprintf("%.2f", result.AvgInputTokens),
			fmt.Sprintf("%.2f", result.AvgOutputTokens),
		}

		for i, v := range data {
			cell, _ := excelize.CoordinatesToCellName(i+1, row)
			e.file.SetCellValue(sheetName, cell, v)
		}
		row++
	}

	// 设置列宽
	for i := range headers {
		col, _ := excelize.ColumnNumberToName(i + 1)
		e.file.SetColWidth(sheetName, col, col, 15)
	}

	return nil
}

// createDetailSheet 创建详细数据表
func (e *XLSXExporter) createDetailSheet(results map[string]*engine.TestResult) error {
	sheetName := "详细数据"
	_, err := e.file.NewSheet(sheetName)
	if err != nil {
		return err
	}

	// 设置标题行
	headers := []string{
		"模型名称", "并发度", "上下文Token", "总时长(s)", "总输入Token", "总输出Token", "总Token",
		"P50延迟(ms)", "P90延迟(ms)", "P95延迟(ms)", "P99延迟(ms)",
		"P50首Token(ms)", "P90首Token(ms)", "P95首Token(ms)", "P99首Token(ms)",
	}

	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		e.file.SetCellValue(sheetName, cell, h)
	}

	// 设置标题行样式
	headerStyle, _ := e.file.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"70AD47"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center"},
	})
	e.file.SetRowStyle(sheetName, 1, 1, headerStyle)

	// 填充数据
	row := 2
	for _, result := range results {
		data := []interface{}{
			result.ModelName,
			result.ConcurrencyLevel,
			result.ContextTargetTokens,
			fmt.Sprintf("%.2f", result.TotalDuration.Seconds()),
			result.InputTokens,
			result.OutputTokens,
			result.TotalTokens,
		}

		// 添加延迟百分位数据
		percentiles := []int{50, 90, 95, 99}
		for _, p := range percentiles {
			if v, ok := result.LatencyPercentiles[p]; ok {
				data = append(data, fmt.Sprintf("%.2f", float64(v.Milliseconds())))
			} else {
				data = append(data, "-")
			}
		}

		// 添加首Token延迟百分位数据
		for _, p := range percentiles {
			if v, ok := result.FirstTokenPercentiles[p]; ok {
				data = append(data, fmt.Sprintf("%.2f", float64(v.Milliseconds())))
			} else {
				data = append(data, "-")
			}
		}

		for i, v := range data {
			cell, _ := excelize.CoordinatesToCellName(i+1, row)
			e.file.SetCellValue(sheetName, cell, v)
		}
		row++
	}

	// 设置列宽
	for i := range headers {
		col, _ := excelize.ColumnNumberToName(i + 1)
		e.file.SetColWidth(sheetName, col, col, 15)
	}

	return nil
}

// createLatencySheet 创建延迟分布表
func (e *XLSXExporter) createLatencySheet(results map[string]*engine.TestResult) error {
	sheetName := "延迟分布"
	_, err := e.file.NewSheet(sheetName)
	if err != nil {
		return err
	}

	// 设置标题行
	headers := []string{"模型名称", "并发度", "请求序号", "延迟(ms)", "首Token延迟(ms)"}

	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		e.file.SetCellValue(sheetName, cell, h)
	}

	// 设置标题行样式
	headerStyle, _ := e.file.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"ED7D31"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center"},
	})
	e.file.SetRowStyle(sheetName, 1, 1, headerStyle)

	// 填充数据
	row := 2
	for _, result := range results {
		maxLen := len(result.AllLatencies)
		if len(result.FirstTokenLatencies) > maxLen {
			maxLen = len(result.FirstTokenLatencies)
		}

		for i := 0; i < maxLen; i++ {
			data := []interface{}{
				result.ModelName,
				result.ConcurrencyLevel,
				i + 1,
			}

			if i < len(result.AllLatencies) {
				data = append(data, fmt.Sprintf("%.2f", float64(result.AllLatencies[i].Milliseconds())))
			} else {
				data = append(data, "-")
			}

			if i < len(result.FirstTokenLatencies) {
				data = append(data, fmt.Sprintf("%.2f", float64(result.FirstTokenLatencies[i].Milliseconds())))
			} else {
				data = append(data, "-")
			}

			for j, v := range data {
				cell, _ := excelize.CoordinatesToCellName(j+1, row)
				e.file.SetCellValue(sheetName, cell, v)
			}
			row++
		}
	}

	// 设置列宽
	for i := range headers {
		col, _ := excelize.ColumnNumberToName(i + 1)
		e.file.SetColWidth(sheetName, col, col, 18)
	}

	return nil
}

// SaveToFile 保存到文件
func (e *XLSXExporter) SaveToFile(filePath string) error {
	return e.file.SaveAs(filePath)
}

// Close 关闭文件
func (e *XLSXExporter) Close() error {
	return e.file.Close()
}

// GenerateXLSXFileName 生成XLSX文件名
func GenerateXLSXFileName() string {
	return fmt.Sprintf("test_result_%s.xlsx", time.Now().Format("20060102_150405"))
}

// GetXLSXFilePath 获取XLSX文件完整路径
func GetXLSXFilePath(machineID, backendID string) string {
	var resultsDir string
	if backendID != "" {
		resultsDir = getBackendResultsDir(machineID, backendID)
	} else {
		resultsDir = getMachineResultsDir(machineID)
	}
	return filepath.Join(resultsDir, GenerateXLSXFileName())
}

// SaveTestResultsToXLSX 保存测试结果到XLSX文件
func SaveTestResultsToXLSX(results map[string]*engine.TestResult, machineID, backendID, machineName string) (string, error) {
	exporter := NewXLSXExporter()
	defer exporter.Close()

	if err := exporter.ExportTestResults(results, machineID, machineName); err != nil {
		return "", fmt.Errorf("导出测试结果失败: %w", err)
	}

	// 确保目录存在
	var resultsDir string
	if backendID != "" {
		resultsDir = getBackendResultsDir(machineID, backendID)
	} else {
		resultsDir = getMachineResultsDir(machineID)
	}
	if err := os.MkdirAll(resultsDir, 0755); err != nil {
		return "", fmt.Errorf("创建目录失败: %w", err)
	}

	filePath := GetXLSXFilePath(machineID, backendID)
	if err := exporter.SaveToFile(filePath); err != nil {
		return "", fmt.Errorf("保存XLSX文件失败: %w", err)
	}

	return filePath, nil
}

