package version

import (
	"fmt"
	"runtime"
)

// 版本信息变量，通过构建时 -ldflags 注入
var (
	Version   = "dev"             // 应用版本号
	Commit    = "none"            // Git 提交哈希
	Date      = "unknown"         // 构建日期
	GoVersion = runtime.Version() // Go 运行时版本
)

// String 返回格式化的版本信息字符串
func String() string {
	return fmt.Sprintf("Version: %s\nCommit: %s\nDate: %s\nGoVersion: %s", Version, Commit, Date, GoVersion)
}

func Print() {
	fmt.Println(String())
}
