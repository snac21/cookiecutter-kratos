package utils

import (
	"fmt"
	"time"
)

// time.ParseInLocation
func TimeToTs(timeStr string) (int64, error) {
	// 定义常见的时间格式
	layouts := []string{
		"2006-01-02 15:04:05",
		"2006-01-02 15:04:05.999",
		"2006-01-02 15:04",
		"2006-01-02",
		"2006/01/02 15:04:05",
		"2006/01/02 15:04",
		"2006/01/02",
		time.RFC3339,
		time.RFC3339Nano,
	}

	// 加载北京时区
	beijingLoc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return 0, fmt.Errorf("加载时区失败: %v", err)
	}

	var t time.Time
	var parsed bool

	// 尝试所有格式
	for _, layout := range layouts {
		t, err = time.ParseInLocation(layout, timeStr, beijingLoc)
		if err == nil {
			parsed = true
			break
		}
	}

	if !parsed {
		return 0, fmt.Errorf("时间格式解析失败: %s", timeStr)
	}

	// 转换为毫秒时间戳
	return t.UnixMilli(), nil
}

// 时间戳转字符串时间（指定时区，北京时间）
func TsToTime(timestamp int64) string {
	beijingLoc, _ := time.LoadLocation("Asia/Shanghai")

	if timestamp > 1e15 { // 纳秒级
		return time.Unix(0, timestamp).In(beijingLoc).Format("2006-01-02 15:04:05.000000000")
	} else if timestamp > 1e12 { // 毫秒级
		sec := timestamp / 1000
		nsec := (timestamp % 1000) * 1e6
		return time.Unix(sec, nsec).In(beijingLoc).Format("2006-01-02 15:04:05.000")
	} else { // 秒级
		return time.Unix(timestamp, 0).In(beijingLoc).Format("2006-01-02 15:04:05")
	}
}
