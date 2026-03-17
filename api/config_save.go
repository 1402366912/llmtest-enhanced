package api

import (
	"log"
	"os"

	"gopkg.in/yaml.v3"
)

// saveConfig 保存配置到文件
func (s *Server) saveConfig() error {
	// 调试：打印即将保存的模型数据
	for i, m := range s.config.Models {
		log.Printf("保存模型[%d]: Name=%s, Type=%s, APIKey=%s, BaseURL=%s",
			i, m.Name, m.Type, m.APIKey, m.BaseURL)
	}

	data, err := yaml.Marshal(s.config)
	if err != nil {
		log.Printf("YAML 序列化失败: %v", err)
		return err
	}

	// 调试：打印 YAML 内容的前几行
	lines := string(data)
	if len(lines) > 500 {
		log.Printf("YAML 内容预览（前500字符）: %s...", lines[:500])
	} else {
		log.Printf("YAML 内容: %s", lines)
	}

	if err := os.WriteFile(s.configPath, data, 0644); err != nil {
		log.Printf("写入文件失败: %v", err)
		return err
	}

	log.Printf("配置已成功保存到: %s", s.configPath)
	return nil
}
