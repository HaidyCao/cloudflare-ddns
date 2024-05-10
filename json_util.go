package main

import (
	"strings"
)

// 根据 keyPath 获取嵌套值
func getNestedValue(data map[string]interface{}, keyPath string) (interface{}, bool) {
	keys := parseKeyPath(keyPath)
	current := data
	for _, key := range keys {
		val, exists := current[key]
		if !exists {
			return nil, false
		}
		if nested, ok := val.(map[string]interface{}); ok {
			current = nested
		} else {
			return val, true
		}
	}
	return nil, false
}

// 解析 keyPath，返回每一级的键名
func parseKeyPath(keyPath string) []string {
	return strings.Split(keyPath, ".")
}
